package synology

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"photoserver/config"
	"time"
)

type NasObject struct {
	Data    interface{} `json:"data"`
	Success bool        `json:"success"`
	Error   *NasError   `json:"error"`
}

type NasSid struct {
	Sid string `json:"sid"`
}

type NasFiles struct {
	Files []NasFile `json:"files"`
}

type NasFile struct {
	IsDir bool   `json:"isdir"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

type NasError struct {
	Code int `json:"code"`
}

type NasConfig struct {
	Host     string
	Port     string
	User     string
	Password string
}

func getConfig() NasConfig {
	return NasConfig{
		Host:     config.GetEnv("SYN_HOST", ""),
		Port:     config.GetEnv("SYN_PORT", ""),
		User:     config.GetEnv("SYN_USER", ""),
		Password: config.GetEnv("SYN_PSSWD", ""),
	}
}

var client = http.Client{
	Timeout: 60 * time.Second,
}

func GetToken() (string, error) {
	synConfig := getConfig()
	request := "http://" +
		synConfig.Host + ":" +
		synConfig.Port +
		"/webapi/auth.cgi?" +
		"api=SYNO.API.Auth&" +
		"version=3&" +
		"method=login&" +
		"account=" + synConfig.User + "&" +
		"passwd=" + synConfig.Password + "&" +
		"session=FileStation&" +
		"format=sid"

	resp, err := client.Get(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyString := string(bodyBytes)

	nasObject := NasObject{
		Data: &NasSid{},
	}
	_ = json.Unmarshal([]byte(bodyString), &nasObject)
	sid := nasObject.Data.(*NasSid)
	return sid.Sid, nil
}

func List(token string, directory string) ([]NasFile, error) {
	synConfig := getConfig()
	request := "http://" +
		synConfig.Host + ":" + synConfig.Port +
		"/webapi/entry.cgi?" +
		"api=SYNO.FileStation.List&" +
		"version=2&" +
		"method=list&" +
		"folder_path=" + directory + "&" +
		"session=FileStation&" +
		"additional=real_path&" +
		"_sid=" + token

	resp, err := client.Get(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	bodyString := string(bodyBytes)

	nasObject := NasObject{
		Data: &NasFiles{
			Files: []NasFile{},
		},
	}
	err = json.Unmarshal([]byte(bodyString), &nasObject)
	if err != nil {
		return nil, err
	}
	files := nasObject.Data.(*NasFiles)
	return files.Files, nil
}

func RecursiveList(token string, baseDirectory string) ([]NasFile, error) {
	var allFiles []NasFile
	err := recursiveList(token, baseDirectory, &allFiles)
	if err != nil {
		return nil, err
	}
	return allFiles, nil
}

func recursiveList(token string, directory string, allFiles *[]NasFile) error {
	files, err := List(token, directory)
	if err != nil {
		return err
	}
	for _, file := range files {
		*allFiles = append(*allFiles, file)
		fmt.Println("Append file: " + file.Path)
		if file.IsDir {
			err := recursiveList(token, file.Path, allFiles)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func DownloadFileFromNas(token string, path string) (buf []byte, err error) {
	synConfig := getConfig()
	request := "http://" +
		synConfig.Host + ":" + synConfig.Port +
		"/webapi/entry.cgi?" +
		"api=SYNO.FileStation.Download&" +
		"version=2&" +
		"method=download&" +
		"path=" + url.QueryEscape(path) + "&" +
		"mode=download&" +
		"_sid=" + token
	resp, err := client.Get(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	//if resp.Status ==
	buff := new(bytes.Buffer)
	_, err = buff.ReadFrom(resp.Body)
	if err != nil {
		return nil, err
	}
	return buff.Bytes(), nil
}
