package monitor

import (
	"fmt"
	"github.com/astaxie/beego/logs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/astaxie/beego"
	"github.com/fsnotify/fsnotify"
)

var EventsChan chan fsnotify.Event //监控反馈channel
var DirPath string
var timeCache map[string]time.Time

type Watch struct {
	Watcher *fsnotify.Watcher
}

func init() {
	EventsChan = make(chan fsnotify.Event, 50)
	DirPath = beego.AppConfig.String("dir::path")
	timeCache = make(map[string]time.Time)
}

func timeClear() {
	select {
	case <- time.After(30 * time.Minute):{
		for key,t := range timeCache {
			interval := time.Now().Sub(t)
			if interval.Seconds() > 1 {
				delete(timeCache,key)
			}
		}
	}
	}
}

func (w *Watch) DirMonitor() {
	go timeClear()
	var err error
	w.Watcher, err = fsnotify.NewWatcher()
	if err != nil {
		logs.Error("[SET WATCHER ERROR]\r")
	}
	defer w.Watcher.Close()

	//通过Walk来遍历目录下的所有子目录
	filepath.Walk(DirPath, func(path string, info os.FileInfo, err error) error {
		//判断是否为目录，是目录则添加监控
		if info.IsDir() {
			path, err := filepath.Abs(path)
			if err != nil {
				logs.Error("[SET ABS PATH ERROR]\r")
				return err
			}
			err = w.Watcher.Add(path)
			if err != nil {
				logs.Error("[ADD PATH ERROR] [%s]\r",path)
				return err
			}
			logs.Info("[monitored dir] [%s]\r",path)
			fmt.Println("监控中的目录：", path)
		}
		return nil
	})

	for {
		select {
		//case <- syncSignal:
		//	<- syncSignal
		case event, ok := <-w.Watcher.Events:
			if !ok {
				logs.Error("[WATTCHER.EVENTS CHAN ERROR]\r")
				continue
			}
			if event.Op&fsnotify.Create == fsnotify.Create {
				fileInfo, err := os.Stat(event.Name)
				if err != nil {
					logs.Error("[GET FILEINFO ERROR] [%s]\r",event.Name)
				}
				timeCache[fileInfo.Name()] = time.Now()
				//如果添加了一个新的子目录，将其添加到监控中
				if fileInfo.IsDir() {
					if runtime.GOOS == "windows" {
						if event.Name[strings.LastIndex(event.Name,"\\")+1:] == "新建文件夹" {
							continue
						}
					}else {
						if event.Name[strings.LastIndex(event.Name,"/")+1:] == "未命名文件夹" {
							continue
						}
					}
					err := w.Watcher.Add(event.Name)
					if err != nil {
						logs.Error("[ADD PATH ERROR] [%s]\r",event.Name)
						continue
					}
					logs.Info("[monitored dir] [%s]\r",event.Name)
					fmt.Println("监控中的目录：", event.Name)
				} else {
					//对创建的文件进行处理
					logs.Info("[创建文件] [%s]\r",event.Name)
					//fmt.Println("创建文件：", event.Name)
					EventsChan <- event
				}
			}
			if event.Op&fsnotify.Write == fsnotify.Write {
				fileInfo, err := os.Stat(event.Name)
				if err != nil {
					logs.Error("[GET FILEINFO ERROR] [%s]\r",event.Name)
				}
				if uploadTime,ok := timeCache[fileInfo.Name()];ok{
					interval := time.Now().Sub(uploadTime)
					if interval.Seconds() <= 1 {
						continue
					}
				}else {
					timeCache[fileInfo.Name()] = time.Now()
				}
				//无视写子目录的情况，只针对修改了的文件操作
				if !fileInfo.IsDir() {
					logs.Info("[写文件] [%s]\r",event.Name)
					//fmt.Println("写文件：", event.Name)
					EventsChan <- event
				}
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				err := w.Watcher.Remove(event.Name)
				if err == nil {
					fmt.Println("删除监控：", event.Name)
				}
				logs.Info("[删除文件] [%s]\r",event.Name)
				//fmt.Println("删除文件:", event.Name)
				EventsChan <- event
			}
			if event.Op&fsnotify.Rename == fsnotify.Rename {
				err := w.Watcher.Remove(event.Name)
				if err == nil {
					fmt.Println("重命名文件：",event.Name)
				}
				//fmt.Println("重命名文件:", event.Name)
				logs.Info("[重命名文件] [%s]\r",event.Name)
				EventsChan <- event
			}
		case err, ok := <-w.Watcher.Errors:
			if !ok {
				logs.Error("[WATCHER CHAN ERROR]")
				return
			}
			fmt.Println(err)
		}
	}
}
