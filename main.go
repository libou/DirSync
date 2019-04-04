package main

import (
	"DirSyncSystem/logFile"
	"DirSyncSystem/monitor"
	"DirSyncSystem/transfer"
	"github.com/fsnotify/fsnotify"
)

var channel chan int
//var syncSignal chan bool

func init() {
	logFile.InitLog()
	channel = make(chan int,50)
	//syncSignal = transfer.SyncSignal
}

func main() {
	////检测云端与PC端文件是否一致，以云端为准进行同步
	transfer.SyncFile()
	//
	//实时监测目录变化
	w := monitor.Watch{}
	go w.DirMonitor()
	go transfer.TimeClean()

	//对目录变化进行处理
	for {
		select {
		case event := <-monitor.EventsChan:
			switch event.Op {
			case fsnotify.Create:
				//上传
				go func() {
					channel <- 1
					transfer.TcpUploadFile(event.Name)
					<- channel
				}()
				//go transfer.TcpUploadFile(event.Name)
			case fsnotify.Write:
				//上传替换
				go func() {
					channel <- 1
					transfer.TcpUploadFile(event.Name)
					<- channel
				}()
				//go transfer.TcpUploadFile(event.Name)
			case fsnotify.Remove:
				//删除
				go func() {
					channel <- 1
					transfer.TcpDeleteFile(event.Name)
					<- channel
				}()
				//go transfer.TcpDeleteFile(event.Name)
			case fsnotify.Rename:
				//删除原名称文件
				//上传新名称文件
				go func() {
					channel <- 1
					transfer.TcpDeleteFile(event.Name)
					<- channel
				}()
				//go transfer.TcpDeleteFile(event.Name)
			}
		}
	}
}
