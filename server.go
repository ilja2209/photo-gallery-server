package main

/*
Environment variables:
SYN_HOST  Host of synology server
SYN_PORT  Port of synology server
SYN_USER  User of synology with rights to watch photos
SYN_PSSWD Password of synology user
PORT Port of this service
BASE_DIRECTORY Base directory of synology where photos are located. From this directory recursive indexation will be performed
SCALE_WIDTH New width of indexed image
SCALE_HEIGHT New height of indexed image
DB_URI Uri of mongodb
DB_LOGIN Login of mongodb user
DB_PSSWD Password of mongodb user
INDEXED_IMG_TMP_DIR Directory where indexed files will be located
*/

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/nfnt/resize"
	"go.mongodb.org/mongo-driver/mongo"
	"image"
	"image/jpeg"
	_ "image/jpeg"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"photoserver/config"
	"photoserver/db"
	"photoserver/synology"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	PREPARING = iota
	RUNNING
	STOPPED
	FINISHED
)

type ScalingConfig struct {
	Width  float64
	Height float64
}

type ConnectionConfig struct {
	Port string
}

type PhotoServerConfig struct {
	ConnectionConf      ConnectionConfig
	BaseDirectory       string
	ScaleConf           ScalingConfig
	IndexedImagesTmpDir string
}

type IndexationStatus struct {
	GeneralNumberOfFiles int `json:"general_number_of_files"`
	CurrentFileNumber    int `json:"current_file_number"`
	IndexationOperation  int `json:"indexation_operation"`
}

type ImageData struct {
	Id string `json:"id"`
}

var token string
var serverConfig PhotoServerConfig
var GlobalIndexationStatus atomic.Value
var dbClient *mongo.Client

func indexationStatusHandler(writer http.ResponseWriter, request *http.Request) {
	status := GlobalIndexationStatus.Load()
	if status == nil {
		GlobalIndexationStatus.Store(IndexationStatus{
			GeneralNumberOfFiles: 0,
			CurrentFileNumber:    0,
			IndexationOperation:  STOPPED,
		})
		status = GlobalIndexationStatus.Load()
	}
	writer.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(writer).Encode(status)
	if err != nil {
		fmt.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(err.Error()))
		return
	}
}

func indexationHandler(writer http.ResponseWriter, request *http.Request) {
	status := GlobalIndexationStatus.Load()
	if status != nil &&
		(status.(IndexationStatus).IndexationOperation == PREPARING ||
			status.(IndexationStatus).IndexationOperation == RUNNING) {
		writer.WriteHeader(http.StatusAlreadyReported)
		return
	}
	go indexation()
	writer.WriteHeader(http.StatusCreated)
}

func indexation() {
	GlobalIndexationStatus.Store(IndexationStatus{
		GeneralNumberOfFiles: 0,
		CurrentFileNumber:    0,
		IndexationOperation:  PREPARING,
	})
	files, _ := synology.RecursiveList(token, serverConfig.BaseDirectory)
	queue := make(chan synology.NasFile, 12)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go process(&wg, queue)
	}

	filteredFiles := filter(files, isImage)

	for fileNumber, file := range filteredFiles {
		GlobalIndexationStatus.Store(IndexationStatus{
			GeneralNumberOfFiles: len(filteredFiles),
			CurrentFileNumber:    fileNumber,
			IndexationOperation:  RUNNING,
		})
		queue <- file
	}
	close(queue)

	wg.Wait()
	terminateIndexation()
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
		(strings.Contains(strings.ToLower(file.Name), ".jpg") ||
			strings.Contains(strings.ToLower(file.Name), ".jpeg"))
}

func process(wg *sync.WaitGroup, queue chan synology.NasFile) {
	defer wg.Done()

	for {
		file, ok := <-queue
		if !ok {
			return
		}
		//Make db record here
		id, err := db.CreateImageDocument(dbClient, file.Path, false)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Println("DB record was added with id: " + id)
		err = processImage(file, id)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func terminateIndexation() {
	GlobalIndexationStatus.Store(IndexationStatus{
		GeneralNumberOfFiles: 100, //100%
		CurrentFileNumber:    100,
		IndexationOperation:  FINISHED,
	})
}

func processImage(file synology.NasFile, key string) error {

	fmt.Println("Process file: " + file.Path)

	fmt.Println("Download file: " + file.Path)
	img, err := synology.DownloadFileFromNas(token, file.Path)
	if err != nil {
		return err
	}

	fmt.Println("Resize image: " + file.Path)
	resizedImg, err := resizeImage(img, serverConfig.ScaleConf.Width, serverConfig.ScaleConf.Height)
	if err != nil {
		return err
	}

	//go describe (find faces e.t.c) - unblocking operation to AI service

	fmt.Println("Save file: " + serverConfig.IndexedImagesTmpDir + key)
	err = ioutil.WriteFile(serverConfig.IndexedImagesTmpDir+key, resizedImg, 0644)
	if err != nil {
		return err
	}

	err = db.UpdateIndexationStatus(dbClient, key, true)
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

func randomImageHandler(writer http.ResponseWriter, request *http.Request) {
	imgId, err := db.GetRandomImageDocument(dbClient)
	if err != nil {
		fmt.Println(err)
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(err.Error()))
		return

	}

	writer.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(writer).Encode(ImageData{Id: imgId})
	if err != nil {
		fmt.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(err.Error()))
		return
	}
}

func getImageHandler(writer http.ResponseWriter, request *http.Request) {
	vars := mux.Vars(request)
	id := vars["id"]
	writer.Header().Set("Content-Type", "image/jpeg")
	file, err := os.Open(serverConfig.IndexedImagesTmpDir + id)
	if err != nil {
		fmt.Println(err)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(err.Error()))
		return
	}
	defer file.Close()
	data, readErr := ioutil.ReadAll(bufio.NewReader(file))
	if readErr != nil {
		fmt.Println(readErr)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(readErr.Error()))
		return
	}
	_, err = writer.Write(data)
	if err != nil {
		fmt.Println(readErr)
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(readErr.Error()))
		return
	}
}

func main() {
	synToken, err := synology.GetToken()
	if err != nil {
		panic(err)
	}
	token = synToken
	fmt.Print(token)

	uri := config.GetEnvOrPanic("DB_URI")
	login := config.GetEnvOrPanic("DB_LOGIN")
	psswd := config.GetEnvOrPanic("DB_PSSWD")

	dbClient, err = db.NewMongoClient(uri, login, psswd)
	if err != nil {
		panic(err)
	}

	fmt.Println("Connection to DB is established")

	serverConfig = PhotoServerConfig{
		ConnectionConf: ConnectionConfig{Port: config.GetEnv("PORT", "8080")},
		BaseDirectory:  config.GetEnv("BASE_DIRECTORY", "/photo"),
		ScaleConf: ScalingConfig{
			Width:  config.GetEnvAsFloat64("SCALE_WIDTH", 0.0),
			Height: config.GetEnvAsFloat64("SCALE_HEIGHT", 0.0),
		},
		IndexedImagesTmpDir: config.GetEnvOrPanic("INDEXED_IMG_TMP_DIR"),
	}

	router := mux.NewRouter().StrictSlash(true)

	router.HandleFunc("/api/v1/photos/{id}/image", getImageHandler).Methods("GET")
	router.HandleFunc("/api/v1/photos", randomImageHandler).Methods("GET") //returns random image
	router.HandleFunc("/api/v1/index", indexationHandler).Methods("POST")  // params: full=true/false. Unblocking operation
	router.HandleFunc("/api/v1/index/status", indexationStatusHandler).Methods("GET")
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
