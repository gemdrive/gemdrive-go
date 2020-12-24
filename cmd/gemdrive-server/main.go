package main

import (
	"encoding/json"
	"flag"
	"fmt"
	gemdrive "github.com/gemdrive/gemdrive-go"
	"io/ioutil"
	"log"
	"os"
	"path"
)

func main() {
	port := flag.Int("port", 3838, "Port")
	var dirs arrayFlags
	flag.Var(&dirs, "dir", "Directory to add")
	gemCacheDir := flag.String("meta-dir", "./gemdrive", "Gem directory")
	rclone := flag.String("rclone", "", "Enable rclone proxy")
	flag.Parse()

	var config *gemdrive.Config
	configBytes, err := ioutil.ReadFile("gemdrive_config.json")
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		log.Fatal(err)
	}

	multiBackend := gemdrive.NewMultiBackend()

	for _, dir := range dirs {
		dirName := path.Base(dir)
		gemDir := path.Join(*gemCacheDir, dirName)
		fsBackend, err := gemdrive.NewFileSystemBackend(dir, gemDir)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		multiBackend.AddBackend(path.Base(dir), fsBackend)
	}

	if *rclone != "" {
		rcloneBackend := gemdrive.NewRcloneBackend()
		multiBackend.AddBackend(*rclone, rcloneBackend)
	}

	//dir := dirs[0]
	//dirName := path.Base(dir)
	//gemDir := path.Join(*gemCacheDir, dirName)
	//fmt.Println(dir, gemDir)
	//fsBackend, err := gemdrive.NewFileSystemBackend(dir, gemDir)
	auth, err := gemdrive.NewAuth(*gemCacheDir, config)

	server := gemdrive.NewServer(config, *port, multiBackend, auth)
	//server := NewServer(*port, fsBackend, auth)
	server.Run()
}

// Taken from https://stackoverflow.com/a/28323276/943814
type arrayFlags []string

func (i *arrayFlags) String() string {
	return "my string representation"
}

func (i *arrayFlags) Set(value string) error {
	*i = append(*i, value)
	return nil
}
