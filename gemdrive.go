package main

import (
        "io"
)

type Item struct {
        Size int64 `json:"size"`
        Children map[string]*Item `json:"children"`
}

type Backend interface {
        List(path string) (*Item, error)
        Get(path string, offset, length int64) (io.Reader, error)
}
