package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/akamensky/base58"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/sha3"
)

type FileInfo struct {
	FileName string `json:"filename"`
	filePath string
	Hash     string `json:"hash"`
}

func HasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}

func ReadLinesFromFile(filePath string) ([]string, error) {
	var lines []string

	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func ReadLinesFromString(input string) ([]string, error) {
	var lines []string

	reader := strings.NewReader(input)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := scanner.Text()
		line = strings.TrimSuffix(line, "\n")
		line = strings.TrimSuffix(line, "\r")
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return lines, nil
}

func Sha3SumFile(file io.Reader) string {
	hash := sha3.New224()
	_, err := io.Copy(hash, file)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func ReadOrCreateFile(filename string, content []byte) ([]byte, error) {
	if _, err := os.Stat(filename); err == nil {
		data, err := os.ReadFile(filename)
		if err != nil {
			return []byte{}, fmt.Errorf("不能读取文件: %v", err)
		}
		return data, nil
	} else if os.IsNotExist(err) {
		err = os.WriteFile(filename, []byte(content), 0755)
		if err != nil {
			return []byte{}, fmt.Errorf("不能创建文件: %v", err)
		}
		return content, nil
	} else {
		return []byte{}, fmt.Errorf("检查文件出错: %v", err)
	}
}

func Base58Encode(input []byte) string {
	return base58.Encode(input)
}

func Base58Decode(input string) (string, error) {
	decoded, err := base58.Decode(input)
	if err != nil {
		logrus.Error(err)
		return "", err
	}
	return string(decoded), nil
}
