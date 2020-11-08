package main

import (
	"bytes"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"photoserver/config"
	"photoserver/synology"
	"strings"
	"time"
)

const (
	RUNNING = iota
	STOPPED
)

type ScalingConfig struct {
	Width  float64
	Height float64
}

type ConnectionConfig struct {
	Port string
}

type PhotoServerConfig struct {
	ConnectionConf ConnectionConfig
	BaseDirectory  string
	ScaleConf      ScalingConfig
}

var token string
var serverConfig PhotoServerConfig

func indexingHandler(writer http.ResponseWriter, request *http.Request) {
	go index()
}

func indexingStatusHandler(writer http.ResponseWriter, request *http.Request) {

}

func index() {
	files, _ := synology.RecursiveList(token, serverConfig.BaseDirectory)
	queue := make(chan synology.NasFile, 12)
	for i := 0; i < 5; i++ {
		go process(queue)
	}

	for _, file := range filter(files, isImage) {
		queue <- file
	}
	close(queue)
}

func filter(files []synology.NasFile, predicate func(file synology.NasFile) bool) []synology.NasFile {
	var filteredSlice []synology.NasFile
	for _, file := range files {
		if predicate(file) {
			filteredSlice = append(filteredSlice, file)
		}
	}
	return filteredSlice
}

func isImage(file synology.NasFile) bool {
	return !file.IsDir &&
		(strings.Contains(file.Name, ".jpg") ||
			strings.Contains(file.Name, ".JPG") ||
			strings.Contains(file.Name, ".jpeg"))
}

func process(queue chan synology.NasFile) {
	for {
		file := <-queue
		//Make db record here
		err := processImage(file)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func processImage(file synology.NasFile) error {
	//download
	//resize
	//describe (find faces e.t.c)
	fmt.Println("Process file: " + file.Path)
	err := downloadAndSave(file)
	if err != nil {
		return err
	}
	return nil
}

func downloadAndSave(file synology.NasFile) error {
	fmt.Println("Download file: " + file.Path)
	data, err := synology.DownloadFileFromNas(token, file.Path)
	if err != nil {
		return err
	}
	resizedImage, err := resizeImage(data, serverConfig.ScaleConf.Width, serverConfig.ScaleConf.Height)
	if err != nil {
		return err
	}
	fmt.Println("Save file: " + "C:\\temp\\" + file.Path + file.Name)
	err = ioutil.WriteFile("C:\\temp\\"+file.Name, resizedImage, 0644)
	if err != nil {
		return err
	}
	return nil
}

func resizeImage(data []byte, w float64, h float64) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}

	scale := w / float64(img.Bounds().Dx())
	newWidth, newHeight := imgScale(img.Bounds(), scale)
	newImage := resize.Resize(newWidth, newHeight, img, resize.Lanczos3)
	options := &jpeg.Options{
		Quality: 50,
	}
	buf := new(bytes.Buffer)
	err = jpeg.Encode(buf, newImage, options)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func imgScale(image image.Rectangle, scale float64) (width, height uint) {
	return uint(float64(image.Dx()) * scale), uint(float64(image.Dy()) * scale)
}

func main() {
	synToken, err := synology.GetToken()
	if err != nil {
		panic(err)
	}
	token = synToken
	fmt.Print(token)

	serverConfig = PhotoServerConfig{
		ConnectionConf: ConnectionConfig{Port: config.GetEnv("PORT", "8080")},
		BaseDirectory:  config.GetEnv("BASE_DIRECTORY", "/photo"),
		ScaleConf: ScalingConfig{
			Width:  config.GetEnvAsFloat64("SCALE_WIDTH", 0.0),
			Height: config.GetEnvAsFloat64("SCALE_HEIGHT", 0.0),
		},
	}

	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/api/v1/photos/{id}/image", func(writer http.ResponseWriter, request *http.Request) {}).Methods("GET")
	router.HandleFunc("/api/v1/index", indexingHandler).Methods("POST") // params: full=true/false. Unblocking operation
	router.HandleFunc("/api/v1/index/status", indexingStatusHandler).Methods("GET")
	router.HandleFunc("/api/v1/photos/{id}", func(writer http.ResponseWriter, request *http.Request) {}).Methods("GET") // returns info about the photo
	router.HandleFunc("/api/v1/photos", func(writer http.ResponseWriter, request *http.Request) {}).Methods("GET")      // returns list all photos
	router.HandleFunc("/api/v1/index", func(writer http.ResponseWriter, request *http.Request) {}).Methods("GET")       // returns index status

	router.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "OK")
	}).Methods("GET", "POST")

	srv := &http.Server{
		Handler:      router,
		Addr:         ":" + serverConfig.ConnectionConf.Port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	fmt.Printf("Server is running")
	log.Fatal(srv.ListenAndServe())
}
