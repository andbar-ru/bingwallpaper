/*
Script downloads today's wallpaper from bingwallpaper.com, sets wallpaper and shows message with
wallpaper description. Information about downloaded wallpapers is saved into wpFile. If today's
wallpaper has been downloaded already, script does nothing. If there are missed dates, script
downloads wallpapers at that dates. wpFile's lines have the following format:
YYYYMMDD <wallpaper-file-name> <description>.
*/
package main

import (
	"bufio"
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
	baseURL          = "https://bing.gifposter.com"
	startURL         = "https://bing.gifposter.com/list/new/desc/classic.html"
	localDateLayout  = "20060102"
	remoteDateLayout = "Jan 2, 2006"
)

var (
	imgDir    = fmt.Sprintf("%s/Images/bing-wallpapers", os.Getenv("HOME"))
	wpFile    = fmt.Sprintf("%s/wallpapers", imgDir)
	now       = time.Now()
	today     = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	yesterday = today.AddDate(0, 0, -1)
	lastDate  time.Time
)

func check(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func getResponse(url string) *http.Response {
	response, err := http.Get(url)
	if err != nil {
		log.Panicf("Could not get response from url %s", url)
	}
	if response.StatusCode != 200 {
		log.Panicf("%s: status code error: %d %s", url, response.StatusCode, response.Status)
	}
	return response
}

// Download wallpaper from the url.
func downloadWallpaper(url string) (time.Time, string, string, string) {
	var date time.Time
	var filename, title, description string

	// Transitional page.
	response := getResponse(url)
	defer response.Body.Close()
	root, err := goquery.NewDocumentFromReader(response.Body)
	check(err)

	// Parse the page and fetch href for the next page.
	href, ok := root.Find("a.fl").First().Attr("href")
	if !ok {
		log.Panicf("Could not find href on the transitional page at date %s", date.Format(localDateLayout))
	}
	href = baseURL + href

	// Page with photo.
	response = getResponse(href)
	defer response.Body.Close()
	root, err = goquery.NewDocumentFromReader(response.Body)
	check(err)

	detail := root.Find("div.detail")
	dateStr := detail.Find("time[itemprop='date']").Text()
	date, err = time.Parse(remoteDateLayout, dateStr)
	check(err)

	title = detail.Find("div.title").Text()
	title = strings.TrimSpace(strings.Split(title, "©")[0])

	description = detail.Find("div.description").Text()

	img := root.Find("#bing_wallpaper")
	src, ok := img.Attr("src")
	if !ok {
		log.Panicf("Could not find img src on url %s", url)
	}
	lastSlashIndex := strings.LastIndex(src, "/")
	filename = src[lastSlashIndex+1:]
	filepath := fmt.Sprintf("%s/%s", imgDir, filename)

	// Download image.
	output, err := os.Create(filepath)
	if err != nil {
		log.Panicf("Could not create file %s, err: %s", filepath, err)
	}
	defer output.Close()
	response = getResponse(src)
	defer response.Body.Close()
	_, err = io.Copy(output, response.Body)
	if err != nil {
		log.Panicf("Could not write image to file, err: %s", err)
	}

	return date, filename, title, description
}

// Set wallpaper and show message with description.
func setWallpaper(filename, title, description string) {
	filepath := fmt.Sprintf("%s/%s", imgDir, filename)

	setWallpaperCmd := exec.Command("fbsetbg", "-f", filepath)
	err := setWallpaperCmd.Start()
	check(err)

	msgCmd := exec.Command("zenity", "--info", "--width=600", "--no-markup", "--title", title, "--text", title+"\n\n"+description)
	err = msgCmd.Start()
	check(err)
}

// Save record about wallpaper into file.
func logWallpaper(date time.Time, filename, title, description string) {
	// Escape some characters for sed.
	description = title + ".  " + description
	fixedDescription := description
	fixedDescription = strings.Replace(fixedDescription, "&", `\x26`, -1)
	fixedDescription = strings.Replace(fixedDescription, "'", `\x27`, -1)
	fixedDescription = strings.Replace(fixedDescription, ";", `\x3b`, -1)
	line := fmt.Sprintf("%s %s %s\\n", date.Format(localDateLayout), filename, fixedDescription)
	sedCmd := exec.Command("sed", "-i", fmt.Sprintf("1s;^;%s;", line), wpFile)
	err := sedCmd.Run()
	check(err)

	// Check that first line matches original description.
	f, err := os.Open(wpFile)
	check(err)
	defer f.Close()
	reader := bufio.NewReader(f)
	firstLine, err := reader.ReadString('\n')
	check(err)
	substrings := strings.SplitN(firstLine, " ", 3)
	savedDescription := substrings[len(substrings)-1]
	if strings.TrimSpace(savedDescription) != strings.TrimSpace(description) {
		log.Printf("%s: Original description and saved description are mismatched.", date.Format(localDateLayout))
	}
}

func main() {
	// Create directory if not exists.
	_, err := os.Stat(imgDir)
	if os.IsNotExist(err) {
		err = os.Mkdir(imgDir, 0755)
		check(err)
	}

	// Fetch the last date and, if the last date is today, exit
	_, err = os.Stat(wpFile)
	if os.IsNotExist(err) {
		f, err := os.Create(wpFile)
		check(err)
		_, err = f.WriteString("\n")
		check(err)
		f.Close()
	} else {
		f, err := os.Open(wpFile)
		check(err)
		lastDateBytes := make([]byte, 8) // YYYYMMDD
		_, err = f.Read(lastDateBytes)
		check(err)
		f.Close()

		lastDate, err = time.Parse(localDateLayout, string(lastDateBytes))
		check(err)
		if lastDate == today {
			os.Exit(0)
		}
	}
	if lastDate.IsZero() {
		lastDate = yesterday
	}

	// Page with thumbs.
	response := getResponse(startURL)
	defer response.Body.Close()
	root, err := goquery.NewDocumentFromReader(response.Body)
	check(err)

	thumbs := root.Find("ul.imglist > li")
	if thumbs.Length() == 0 {
		log.Panicf("Could not find thumbs")
	}

	// Collect urls until the last date.
	urls := make([]string, 0)
	thumbs.EachWithBreak(func(i int, thumb *goquery.Selection) bool {
		dateStr := thumb.Find("time").First().Text()
		date, err := time.Parse(remoteDateLayout, dateStr)
		check(err)

		if !date.After(lastDate) {
			return false
		}
		// Tomorrow date may exist but attempt to download wallpaper returns error 404.
		if date.After(today) {
			return true
		}

		href, ok := thumb.Find("a").First().Attr("href")
		if !ok {
			log.Panicf("Could not find url at date %s", date.Format(localDateLayout))
		}
		url := baseURL + href
		urls = append(urls, url)

		return true
	})

	// If there are new urls, range them from last to first.
	if len(urls) > 0 {
		// Except first: only download and log.
		for i := len(urls) - 1; i > 0; i-- {
			date, filename, title, description := downloadWallpaper(urls[i])
			logWallpaper(date, filename, title, description)
		}
		// For the first url further set wallpaper and output message.
		date, filename, title, description := downloadWallpaper(urls[0])
		setWallpaper(filename, title, description)
		logWallpaper(date, filename, title, description)
	}
}
