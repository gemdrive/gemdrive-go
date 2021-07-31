package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"path/filepath"

	"github.com/anderspitman/treemess-go"
	gemdrive "github.com/gemdrive/gemdrive-go"
)

func main() {

	port := flag.Int("port", 0, "Port")
	var dirs arrayFlags
	flag.Var(&dirs, "dir", "Directory to add")
	configPath := flag.String("config", "", "Config path")
	runDir := flag.String("run-dir", "", "Database directory")
	rclone := flag.String("rclone", "", "Enable rclone proxy")
	flag.Parse()

	config := &gemdrive.Config{
		Port: 3838,
		Dirs: []string{},
	}

	if *configPath == "" {
		*configPath = filepath.Join(*runDir, "gemdrive_config.json")
	}

	configBytes, err := ioutil.ReadFile(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(configBytes, &config)
	if err != nil {
		log.Fatal(err)
	}

	if *port != 0 {
		config.Port = *port
	}

	if config.DataDir == "" {
		config.DataDir = filepath.Join(*runDir, "data")
	}

	if config.CacheDir == "" {
		config.CacheDir = filepath.Join(*runDir, "cache")
	}

	if *rclone != "" {
		config.RcloneDir = *rclone
	}

	for _, dir := range dirs {
		config.Dirs = append(config.Dirs, dir)
	}

	tmess := treemess.NewTreeMess()
	gdTmess := tmess.Branch()

	_, err = gemdrive.NewServer(config, gdTmess)
	if err != nil {
		log.Fatal(err)
	}

	ch := make(chan treemess.Message)
	tmess.Listen(ch)

	tmess.Send("start", nil)

	for msg := range ch {
		fmt.Println(msg)
	}
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
