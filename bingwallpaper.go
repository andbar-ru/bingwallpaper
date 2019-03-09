/*
Script downloads today's wallpaper from bingwallpaper.com, sets wallpaper and shows message with
wallpaper description. Information about downloaded wallpapers is saved into WP_FILE. If today's
wallpaper has been downloaded already, script does nothing. If there are missed dates, script
downloads wallpapers at that dates. WP_FILE's lines have the following format:
YYYYMMDD <wallpaper-file-name> <description>.
*/
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
)

const (
	BASE_URL           = "https://bing.gifposter.com"
	LOCAL_DATE_LAYOUT  = "20060102"
	REMOTE_DATE_LAYOUT = "Jan 2, 2006"
)

var (
	IMG_DIR   = fmt.Sprintf("%s/Images/bing-wallpapers", os.Getenv("HOME"))
	WP_FILE   = fmt.Sprintf("%s/wallpapers", IMG_DIR)
	now       = time.Now()
	TODAY     = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	YESTERDAY = TODAY.AddDate(0, 0, -1)
	lastDate  time.Time
)

func check(err error) {
	if err != nil {
		log.Panic(err)
	}
}

// Download wallpaper from the url.
func downloadWallpaper(url string) (time.Time, string, string) {
	var date time.Time
	var filename, description string

	// Fetch the page with photo
	response, err := http.Get(url)
	check(err)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		log.Panicf("%s: status code error: %d %s", url, response.StatusCode, response.Status)
	}

	// Parse the page and fetch the image metadata
	root, err := goquery.NewDocumentFromReader(response.Body)
	check(err)
	detail := root.Find(".detail")
	dateStr := detail.Find("time[itemprop='date']").Text()
	date, err = time.Parse(REMOTE_DATE_LAYOUT, dateStr)
	check(err)

	description = detail.Find(".description").Text()

	img := root.Find("#bing_wallpaper")
	src, ok := img.Attr("src")
	if !ok {
		log.Panicf("Could not find img src on url %s", url)
	}
	src = BASE_URL + src
	lastSlashIndex := strings.LastIndex(src, "/")
	filename = src[lastSlashIndex+1:]
	filepath := fmt.Sprintf("%s/%s", IMG_DIR, filename)

	// Download image
	output, err := os.Create(filepath)
	if err != nil {
		log.Panicf("Could not create file %s, err: %s", filepath, err)
	}
	defer output.Close()
	response, err = http.Get(src)
	if err != nil {
		log.Panicf("Could not download image from %s, err: %s", src, err)
	}
	defer response.Body.Close()
	_, err = io.Copy(output, response.Body)
	if err != nil {
		log.Panicf("Could not write image to file, err: %s", err)
	}

	return date, filename, description
}

// Set wallpaper and show message with description.
func setWallpaper(filename, description string) {
	filepath := fmt.Sprintf("%s/%s", IMG_DIR, filename)

	setWallpaperCmd := exec.Command("fbsetbg", "-f", filepath)
	err := setWallpaperCmd.Start()
	check(err)

	msgCmd := exec.Command("zenity", "--info", "--width=600", "height=400", "--text", description)
	err = msgCmd.Start()
	check(err)
}

// Save record about wallpaper into file.
func logWallpaper(date time.Time, filename, description string) {
	// Escape some characters for sed.
	description = strings.Replace(description, "'", `\x27`, -1)
	description = strings.Replace(description, ";", `\x3b`, -1)
	line := fmt.Sprintf("%s %s %s\\n", date.Format(LOCAL_DATE_LAYOUT), filename, description)
	sedCmd := exec.Command("sed", "-i", fmt.Sprintf("1s;^;%s;", line), WP_FILE)
	err := sedCmd.Start()
	check(err)
}

func main() {
	// Create directory if not exists
	_, err := os.Stat(IMG_DIR)
	if os.IsNotExist(err) {
		err = os.Mkdir(IMG_DIR, 0755)
		check(err)
	}

	// Fetch the last date and, if the last date is today, exit
	_, err = os.Stat(WP_FILE)
	if os.IsNotExist(err) {
		f, err := os.Create(WP_FILE)
		check(err)
		_, err = f.WriteString("\n")
		check(err)
		f.Close()
	} else {
		f, err := os.Open(WP_FILE)
		check(err)
		lastDateBytes := make([]byte, 8) // YYYYMMDD
		_, err = f.Read(lastDateBytes)
		check(err)
		lastDate, err = time.Parse(LOCAL_DATE_LAYOUT, string(lastDateBytes))
		check(err)
		if lastDate == TODAY {
			os.Exit(0)
		}
	}
	if lastDate.IsZero() {
		lastDate = YESTERDAY
	}

	// Page with thumbs.
	response, err := http.Get(BASE_URL)
	check(err)
	defer response.Body.Close()
	if response.StatusCode != 200 {
		log.Panicf("status code error: %d %s", response.StatusCode, response.Status)
	}
	root, err := goquery.NewDocumentFromReader(response.Body)
	check(err)
	thumbs := root.Find("article.thumb")
	if thumbs.Length() == 0 {
		log.Panicf("Could not find thumbs")
	}

	// Collect urls until the last date
	urls := make([]string, 0)
	thumbs.EachWithBreak(func(i int, thumb *goquery.Selection) bool {
		dateStr := thumb.Find("time.date").First().Text()
		date, err := time.Parse(REMOTE_DATE_LAYOUT, dateStr)
		check(err)

		if !date.After(lastDate) {
			return false
		}
		// Tomorrow date may exist but attempt to download wallpaper returns error 404.
		if date.After(TODAY) {
			return true
		}

		href, ok := thumb.Find("a").First().Attr("href")
		if !ok {
			log.Panicf("Could not find url at date %s", date.Format(LOCAL_DATE_LAYOUT))
		}
		url := BASE_URL + href
		urls = append(urls, url)
		return true
	})

	// If there are new urls, range them from the end until the first: only download and log.
	if len(urls) > 0 {
		for i := len(urls) - 1; i > 0; i-- {
			date, filename, description := downloadWallpaper(urls[i])
			logWallpaper(date, filename, description)
		}
		// For the first url further set wallpaper and output message.
		date, filename, description := downloadWallpaper(urls[0])
		setWallpaper(filename, description)
		logWallpaper(date, filename, description)
	}
}
