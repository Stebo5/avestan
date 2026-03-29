package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
	"golang.org/x/time/rate"
)

type FolioGenerator struct {
	counter atomic.Int64
}

func NewFolioGenerator() *FolioGenerator {
	return &FolioGenerator{}
}

func (fg *FolioGenerator) Next() string {
	n := fg.counter.Add(1) - 1

	var suffix string
	if n%2 == 0 {
		suffix = "r"
	} else {
		suffix = "v"
	}

	return fmt.Sprintf("%d%s", n/2+1, suffix)
}

func FailPrintln(a ...any) {
	fmt.Fprintln(os.Stderr, a...)
	os.Exit(1)
}

type WorkerConfig struct {
	Ctx        context.Context
	Client     *http.Client
	BaseURL    string
	Generator  *FolioGenerator
	Wg         *sync.WaitGroup
	Limiter    *rate.Limiter
	Manuscript string
	OutputPath string
}

var workDone atomic.Bool

func Worker(config *WorkerConfig) {
	defer config.Wg.Done()

	for {
		if config.Ctx.Err() != nil {
			return
		}
		if err := config.Limiter.Wait(config.Ctx); err != nil {
			return
		}

		folio := config.Manuscript + "_" + config.Generator.Next()
		url := config.BaseURL + fmt.Sprintf("%s%%2F%s.jpg/full/max/0/default.jpg", config.Manuscript, folio)

		request, _ := http.NewRequestWithContext(config.Ctx, http.MethodGet, url, nil)
		response, err := config.Client.Do(request)
		if err != nil {
			log.Println("Request error:", err)
			return
		}

		if response.StatusCode == http.StatusNotFound {
			response.Body.Close()
			log.Println("Stopping at:", url)
			return
		}

		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			log.Println("Server error:", response.Status, url)
			return
		}

		filename := filepath.Join(config.OutputPath, folio+".jpg")
		file, err := os.Create(filename)
		if err != nil {
			response.Body.Close()
			log.Println("Error creating file:", err)
			return
		}

		_, err = io.Copy(file, response.Body)

		response.Body.Close()
		file.Close()

		if err != nil {
			log.Println("Error writing file:", err)
			os.Remove(filename)
			return
		}

		fmt.Println("Saved: ", filename)
		workDone.Store(true)
	}
}

func GetDownloadsPath() string {
	result, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0)
	if err != nil {
		return ""
	}
	return result
}

func GetConfigPath() string {
	result, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return result
}

var downloadDirectory string
var workerCount int

var downloadCommand = &cobra.Command{
	Use:   "avestan [manuscript]",
	Short: "Download scans from Avestan Digital Archive",
	Long:  "avestan is a tool for downloading scans from Avestan Digital Archive by providing the manuscript.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		manuscript := args[0]
		outputDirectory := filepath.Join(downloadDirectory, "manuscript_"+manuscript)

		err := os.MkdirAll(outputDirectory, os.ModePerm)
		if err != nil {
			FailPrintln("Error creating output directory:", err)
		}
		
		logDirectory := filepath.Join(GetConfigPath(), "avestan", "logs")
		err = os.MkdirAll(logDirectory, os.ModePerm)
		if err != nil {
			FailPrintln("Error creating log directory:", err)
		}

		logPath := filepath.Join(logDirectory, fmt.Sprintf("%s_%d.log", manuscript, time.Now().UnixNano()))
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error creating log file:", err)
		}
		log.SetOutput(logFile)
		log.SetFlags(log.LstdFlags | log.Lshortfile)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var wg sync.WaitGroup

		config := &WorkerConfig{
			Ctx:        ctx,
			Client:     &http.Client{},
			BaseURL:    "https://ada.geschkult.fu-berlin.de/iiif/iiif/3/",
			Generator:  &FolioGenerator{},
			Wg:         &wg,
			Limiter:    rate.NewLimiter(5, 1),
			Manuscript: manuscript,
			OutputPath: outputDirectory,
		}

		for range workerCount {
			wg.Add(1)
			go Worker(config)
		}

		wg.Wait()

		if !workDone.Load() {
			os.RemoveAll(outputDirectory)
		}
	},
}

func Execute() {
	downloadCommand.Flags().StringVarP(&downloadDirectory, "output", "o", GetDownloadsPath(), "Output directory")
	downloadCommand.Flags().IntVarP(&workerCount, "workers", "w", 5, "Number of workers")

	if err := downloadCommand.Execute(); err != nil {
		FailPrintln("Error executing avestan:", err)
	}
}

func main() {
	Execute()
}
