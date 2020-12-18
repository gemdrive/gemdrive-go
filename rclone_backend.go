package gemdrive

import (
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

type RcloneBackend struct {
}

type rcloneItem struct {
	Name    string
	Size    int64
	ModTime string
	IsDir   bool
}

func NewRcloneBackend() *RcloneBackend {
	return &RcloneBackend{}
}

func (b *RcloneBackend) List(reqPath string, maxDepth int) (*Item, error) {
	if reqPath == "/" {
		return b.listRemotes()
	}

	rcloneItems, err := b.rcloneLs(reqPath)
	if err != nil {
		return nil, err
	}

	parentItem := &Item{
		Children: make(map[string]*Item),
	}

	for _, item := range rcloneItems {
		child := &Item{
			Size:    item.Size,
			ModTime: item.ModTime,
		}

		if item.IsDir {
			parentItem.Children[item.Name+"/"] = child
		} else {
			parentItem.Children[item.Name] = child
		}
	}

	return parentItem, nil
}

func (b *RcloneBackend) Read(reqPath string, offset, length int64) (*Item, io.ReadCloser, error) {
	rcloneItems, err := b.rcloneLs(reqPath)
	if err != nil {
		return nil, nil, err
	}

	item := &Item{
		Size:    rcloneItems[0].Size,
		ModTime: rcloneItems[0].ModTime,
	}

	args := []string{"cat"}

	if offset != 0 {
		args = append(args, "--offset", fmt.Sprintf("%d", offset))
	}

	if length != 0 {
		args = append(args, "--count", fmt.Sprintf("%d", length))
	}

	parts := strings.Split(reqPath, "/")
	rclonePath := parts[1] + ":" + strings.Join(parts[2:], "/")

	args = append(args, rclonePath)

	cmd := exec.Command("rclone", args...)

	data, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, nil, err
	}

	return item, data, nil
}

func (b *RcloneBackend) listRemotes() (*Item, error) {
	cmd := exec.Command("rclone", "listremotes")
	stdout, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(stdout), "\n")

	rootItem := &Item{
		Children: make(map[string]*Item),
	}

	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		child := &Item{}
		remoteName := line[:len(line)-1] + "/"
		rootItem.Children[remoteName] = child
	}

	return rootItem, nil
}

func (b *RcloneBackend) rcloneLs(reqPath string) ([]rcloneItem, error) {
	parts := strings.Split(reqPath, "/")
	rclonePath := parts[1] + ":" + strings.Join(parts[2:], "/")
	cmd := exec.Command("rclone", "lsjson", rclonePath)
	stdout, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var rcloneItems []rcloneItem
	err = json.Unmarshal(stdout, &rcloneItems)
	if err != nil {
		return nil, err
	}

	return rcloneItems, nil
}
