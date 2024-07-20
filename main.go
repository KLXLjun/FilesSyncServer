package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	nested "github.com/antonfisher/nested-logrus-formatter"
	"github.com/enriquebris/goconcurrentqueue"
	"github.com/labstack/echo/v4"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/sirupsen/logrus"
)

const SyncVersion = 0

var Quit = make(chan os.Signal)
var Lock = sync.RWMutex{}
var ExecPath = ""
var downloadTaskQueue = goconcurrentqueue.NewFIFO()

func GetDownloadTaskLen() int {
	downloadTaskQueue.Lock()
	defer downloadTaskQueue.Unlock()
	return downloadTaskQueue.GetLen()
}

func PushDownloadTask(key string) {
	downloadTaskQueue.Lock()
	defer downloadTaskQueue.Unlock()
	downloadTaskQueue.Enqueue(key)
}

// check > key > dl
var dlMap = cmap.New[cmap.ConcurrentMap[string, *dl2Hash]]()

type dl2Hash struct {
	FileMap []FileInfo
	dl      map[string]string
	sync.RWMutex
}

func (m *dl2Hash) Init() {
	m.Lock()
	defer m.Unlock()
	m.dl = make(map[string]string, 0)
}

func (m *dl2Hash) Get(key string) string {
	m.RLock()
	defer m.RUnlock()
	if rls, has := m.dl[key]; has {
		return rls
	}
	return ""
}

func (m *dl2Hash) Set(key string, value string) {
	m.Lock()
	defer m.Unlock()
	m.dl[key] = value
}

type Ext map[string]any

func main() {
	logrus.SetLevel(logrus.TraceLevel)
	logrus.SetFormatter(&nested.Formatter{
		HideKeys:        true,
		NoColors:        true,
		TimestampFormat: time.RFC3339,
		FieldsOrder:     []string{"component", "category"},
	})
	logrus.Info("FilesSyncServer")
	logrus.Info(fmt.Sprintf("服务端同步格式版本: %d", SyncVersion))

	ex, err := os.Executable()
	if err != nil {
		logrus.Error("获取执行目录错误", err)
	}
	exPath := filepath.Dir(ex)
	logrus.Info("执行目录:", exPath)
	ExecPath = exPath

	logrus.Trace("读入配置文件")
	LoadConf(exPath)

	databaseDirCreateErr := os.MkdirAll(path.Join(exPath, "database"), 0775)
	if databaseDirCreateErr != nil {
		logrus.Error("创建列表文件夹错误", databaseDirCreateErr)
		os.Exit(0)
	}

	dbFiles, readDBFilesErr := os.ReadDir(path.Join(exPath, "database"))
	if readDBFilesErr != nil {
		logrus.Error("创建列表文件夹错误", readDBFilesErr)
		os.Exit(0)
	}

	if len(dbFiles) == 0 {
		tmp, OpenErr := os.Create(path.Join(exPath, "database", "example.txt"))
		if OpenErr != nil {
			logrus.Warn("创建示例文件失败", OpenErr)
		} else {
			_, writeFileErr := tmp.WriteString("mods")
			if writeFileErr != nil {
				logrus.Warn("写入示例文件失败", writeFileErr)
			}
		}

		downloadDirErr := os.MkdirAll(path.Join(exPath, "dl", "example", "mods"), 0775)
		if downloadDirErr != nil {
			logrus.Error("创建示例下载文件夹错误", downloadDirErr)
		}
	}

	e := echo.New()
	e.GET("/", func(c echo.Context) error {
		return c.JSON(http.StatusOK, &Ext{
			"code": "200",
			"msg":  "ok",
			"ver":  SyncVersion,
		})
	})

	e.GET("/list/:check", func(c echo.Context) error {
		check := c.Param("check")
		if check == "" {
			return c.JSON(http.StatusNotFound, &Ext{
				"check": check,
				"msg":   "请求不正确",
				"ver":   SyncVersion,
			})
		}
		checkFile := c.Param("check") + ".txt"
		if DiskCache.Has(checkFile) {
			readBytes, readError := DiskCache.Read(checkFile)
			if readError != nil {
				return c.JSON(http.StatusInternalServerError, &Ext{
					"check": check,
					"msg":   "服务器不能读取数据",
					"ver":   SyncVersion,
				})
			}
			usePath, ReadErr := ReadLinesFromString(string(readBytes))
			if ReadErr != nil {
				downloadTaskQueue.Dequeue()
				return c.JSON(http.StatusInternalServerError, &Ext{
					"check": check,
					"msg":   "服务器无法解析配置",
					"ver":   SyncVersion,
				})
			}
			rsl := ListResult{
				Ver:    SyncVersion,
				Folder: usePath,
			}
			json.Unmarshal(readBytes, &rsl.Folder)
			return c.JSON(http.StatusOK, &rsl)
		}
		return c.JSON(http.StatusNotFound, &Ext{
			"check": check,
			"msg":   "未找到该查询码,请检查客户端配置",
			"ver":   SyncVersion,
		})
	})

	e.GET("/update/:check/:key", func(c echo.Context) error {
		check := c.Param("check")
		checkFile := c.Param("check") + ".txt"
		key, decErr := Base58Decode(c.Param("key"))
		if decErr != nil {
			return c.JSON(http.StatusBadRequest, &Ext{
				"check": check,
				"key":   c.Param("key"),
				"msg":   "键不正确",
				"ver":   SyncVersion,
			})
		}
		logrus.Debug(fmt.Sprintf("请求查询码:%s 键:%s", check, key))
		for GetDownloadTaskLen() > 0 {
			time.Sleep(time.Millisecond * 50)
		}
		if check == "" || key == "" {
			return c.JSON(http.StatusNotFound, &Ext{
				"check": check,
				"key":   key,
				"msg":   "请求不正确",
				"ver":   SyncVersion,
			})
		}
		if DiskCache.Has(checkFile) {
			readBytes, readError := DiskCache.Read(checkFile)
			if readError != nil {
				return c.JSON(http.StatusInternalServerError, &Ext{
					"check": check,
					"key":   key,
					"msg":   "服务器不能读取数据",
					"ver":   SyncVersion,
				})
			}
			if !dlMap.Has(check) {
				PushDownloadTask(check)
				o1 := cmap.New[*dl2Hash]()
				logrus.Debug(fmt.Sprintf("计算并缓存 %s 中 %s 所有文件哈希值", check, key))
				usePath, ReadErr := ReadLinesFromString(string(readBytes))
				for _, item := range usePath {
					logrus.Info(fmt.Sprintf("正在计算 %s 中 %s 所有文件哈希值", check, key))
					newPackage := dl2Hash{}
					newPackage.Init()
					ol := scan(path.Join(exPath, "dl", check, item))
					for _, md := range ol {
						newPackage.Set(md.Hash, md.filePath)
					}
					newPackage.FileMap = ol
					o1.Set(item, &newPackage)
				}
				dlMap.Set(check, o1)
				if ReadErr != nil {
					downloadTaskQueue.Dequeue()
					return c.JSON(http.StatusInternalServerError, &Ext{
						"check": check,
						"key":   key,
						"msg":   "服务器无法解析配置",
						"ver":   SyncVersion,
					})
				}
				downloadTaskQueue.Dequeue()
				if rls, has := dlMap.Get(check); has {
					if out, hasOk := rls.Get(key); hasOk {
						return c.JSON(http.StatusOK, &Ext{
							"check": check,
							"key":   key,
							"msg":   "成功",
							"file":  out.FileMap,
							"ver":   SyncVersion,
						})
					}
				}
				return c.JSON(http.StatusInternalServerError, &Ext{
					"check": check,
					"key":   key,
					"msg":   "服务器不能编码数据",
					"ver":   SyncVersion,
				})
			} else {
				if rls, has := dlMap.Get(check); has {
					if out, hasOk := rls.Get(key); hasOk {
						return c.JSON(http.StatusOK, &Ext{
							"check": check,
							"key":   key,
							"msg":   "成功",
							"file":  out.FileMap,
							"ver":   SyncVersion,
						})
					}
				}
				return c.JSON(http.StatusInternalServerError, &Ext{
					"check": check,
					"key":   key,
					"msg":   "未知服务器错误",
					"ver":   SyncVersion,
				})
			}
		}
		return c.JSON(http.StatusNotFound, &Ext{
			"check": check,
			"key":   key,
			"msg":   "未找到该查询码,请检查客户端配置",
			"ver":   SyncVersion,
		})
	})

	e.GET("/dl/:check/:key/:hash", func(c echo.Context) error {
		check := c.Param("check")
		checkFile := c.Param("check") + ".txt"
		key, decErr := Base58Decode(c.Param("key"))
		if decErr != nil {
			return c.JSON(http.StatusBadRequest, &Ext{
				"check": check,
				"key":   key,
				"msg":   "键不正确",
				"ver":   SyncVersion,
			})
		}
		fileHash := c.Param("hash")
		logrus.Debug(fmt.Sprintf("下载文件 查询码:%s 键:%s 哈希值:%s", check, key, fileHash))
		if DiskCache.Has(checkFile) {
			if rls, has := dlMap.Get(check); has {
				if out, hasOk := rls.Get(key); hasOk {
					if t := out.Get(fileHash); t != "" {
						c.File(t)
						return nil
					}
				}
			}
		}
		return c.NoContent(http.StatusNotFound)
	})

	logrus.Info("服务器已就绪,运行端口:" + strconv.Itoa(conf.Port))
	e.Logger.Fatal(e.Start(":" + strconv.Itoa(conf.Port)))
}

func scan(scanPath string) []FileInfo {
	logrus.Debug("扫描目录:", scanPath)
	templateFolder, rederr := os.Open(scanPath)
	defer func() {
		_ = templateFolder.Close()
	}()
	if rederr != nil {
		logrus.Error("模板目录扫描失败,原因是:", rederr)
		return nil
	}
	filelist := make([]string, 0)
	templateInfos, _ := templateFolder.Readdir(-1)
	for _, info := range templateInfos {
		if !info.IsDir() {
			filelist = append(filelist, info.Name())
		}
	}

	local := make([]FileInfo, 0)
	for _, s := range filelist {
		file, err := os.Open(path.Join(scanPath, s))
		if err != nil {
			logrus.Error("打开文件失败,原因是:", err)
		}
		resultHash := Sha3SumFile(file)
		file.Close()
		local = append(local, FileInfo{
			FileName: path.Base(path.Join(scanPath, s)),
			filePath: path.Join(scanPath, s),
			Hash:     resultHash,
		})
		time.Sleep(40 * time.Millisecond)
	}
	return local
}
