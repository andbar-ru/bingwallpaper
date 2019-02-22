## Dependencies
* Go compiler
* fbsetbg
* zenity
* sed

Go packages:
* github.com/PuerkitoBio/goquery

## Installation
```
go build -o $HOME/bin/bingwallpaper bingwallpaper.go
```
```
crontab -e
```
Append line `15 * * * * DISPLAY=:0 /home/<user>/bin/bingwallpaper`

