package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
)

const user = ""
const pswd = ""
const host = ""
const port = 5000

type NasObject struct {
	Data    *NasSid `json: "data"`
	Success bool    `json: "success"`
}

type NasSid struct {
	Sid string `json: "sid"`
}

var token string

func getToken() (str string, err error) {
	request := host + ":" + strconv.Itoa(port) + "/webapi/auth.cgi?api=SYNO.API.Auth&version=3&method=login&account=" + user + "&passwd=" + pswd + "&session=FileStation&format=sid"
	resp, err := http.Get(request)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyString := string(bodyBytes)
	fmt.Println(bodyString)
	nasObject := NasObject{
		Data: &NasSid{},
	}
	_ = json.Unmarshal([]byte(bodyString), &nasObject)
	return nasObject.Data.Sid, nil
}

func downloadImageFromNas(path string) (buf []byte, err error) {
	fmt.Println(token)
	if err != nil {
		return nil, err
	}
	request := host + ":" + strconv.Itoa(port) + "/webapi/entry.cgi?api=SYNO.FileStation.Download&version=2&method=download&path=" + url.QueryEscape(path) + "&mode=download&_sid=" + token
	resp, err := http.Get(request)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	//if resp.Status ==
	buff := new(bytes.Buffer)
	buff.ReadFrom(resp.Body)
	return buff.Bytes(), nil
}

func processAndDownloadImageHandler(w http.ResponseWriter, r *http.Request) {
	params := r.URL.Query()
	path := params.Get("path")
	width, err := strconv.ParseUint(params.Get("width"), 10, 32)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "width must be passed", 400)
		return
	}
	height, err := strconv.ParseUint(params.Get("height"), 10, 32)
	if err != nil {
		fmt.Println(err)
		http.Error(w, "height must be passed", 400)
		return
	}
	fmt.Println(path)
	fmt.Println(height)
	// file, err := os.Open("/volume1" + path)
	// defer file.Close()
	// if err != nil {
	// 	http.Error(w, "File not found.", 404)
	// 	return
	// }

	data, err := downloadImageFromNas(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	image, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		fmt.Println(err)
		return
	}

	resizeWidth := float64(width)
	scale := resizeWidth / float64(image.Bounds().Dx())
	newWidth, newHeight := Scale(image.Bounds(), scale)
	newImage := resize.Resize(uint(newWidth), uint(newHeight), image, resize.Lanczos3)
	options := &jpeg.Options{
		Quality: 50,
	}
	w.Header().Set("Content-Type", "image/jpeg")
	err = jpeg.Encode(w, newImage, options)
}

//Scale is ...
func Scale(image image.Rectangle, scale float64) (width, height uint) {
	return uint(float64(image.Dx()) * scale), uint(float64(image.Dy()) * scale)
}

func main() {
	token, _ = getToken()
	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/api/v1/image", processAndDownloadImageHandler).Methods("GET")

	router.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	}).Methods("GET", "POST")

	srv := &http.Server{
		Handler:      router,
		Addr:         ":8080",
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	fmt.Printf("Server is running")
	log.Fatal(srv.ListenAndServe())
}
