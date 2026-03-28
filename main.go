package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/spf13/cobra"
	"golang.org/x/sys/windows"
)

var outputDirectory string

var downloadCommand = &cobra.Command{
	Use:   "avestan [manuscript] [folio]",
	Short: "Download scans from Avestan Digital Archive",
	Long:  "avestan is a tool for downloading scans from Avestan Digital Archive by providing the manuscript and folio.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		manuscript := args[0]
		folio := args[1]
		urlPath := fmt.Sprintf("%s%%2F%s.jpg/full/max/0/default.jpg", manuscript, folio)
		filePath := path.Join(outputDirectory, fmt.Sprintf("%s_%s.jpg", manuscript, folio))

		DownloadImage(urlPath, filePath)
	},
}

func DownloadImage(urlPath string, filePath string) {
	base := "https://ada.geschkult.fu-berlin.de/iiif/iiif/3/"
	url := base + urlPath

	response, err := http.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
		file, err := os.OpenFile(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil && os.IsExist(err) {
			log.Fatalf("File '%s' already exists", filePath)
		}
		defer file.Close()

		_, err = io.Copy(file, response.Body)
		if err != nil {
			log.Fatal(err)
		}
	case http.StatusNotFound:
		log.Fatal("Invalid manuscript or folio")
	default:
		log.Fatal("Error communicating with server")
	}
}

func GetDownloadsPath() string {
	downloadsPath, err := windows.KnownFolderPath(windows.FOLDERID_Downloads, 0)
	if err != nil {
		return ""
	}
	return downloadsPath
}

func Execute() {
	downloadCommand.Flags().StringVarP(&outputDirectory, "output", "o", GetDownloadsPath(), "Output directory")

	if err := downloadCommand.Execute(); err != nil {
		log.Fatalf("Error executing avestan '%s'\n", err)
	}
}

func main() {
	Execute()
}
