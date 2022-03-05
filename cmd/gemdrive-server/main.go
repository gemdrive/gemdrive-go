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

	port := flag.Int("port", 3838, "Port")
	var dirs arrayFlags
	flag.Var(&dirs, "dir", "Directory to add")
	configPath := flag.String("config", "", "Config path")
	runDir := flag.String("run-dir", "", "Database directory")
	rclone := flag.String("rclone", "", "Enable rclone proxy")
	waygateServer := flag.String("waygate-server", "", "Waygate server")
	flag.Parse()

	config := &gemdrive.Config{
		Port:          *port,
		Dirs:          []string{},
		DataDir:       *runDir,
		CacheDir:      filepath.Join(*runDir, "cache"),
		RcloneDir:     *rclone,
		WaygateServer: *waygateServer,
	}

	if *configPath == "" {
		*configPath = filepath.Join(*runDir, "gemdrive_config.json")
	}

	configBytes, err := ioutil.ReadFile(*configPath)
	if err == nil {
		err = json.Unmarshal(configBytes, &config)
		if err != nil {
			log.Fatal(err)
		}
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
