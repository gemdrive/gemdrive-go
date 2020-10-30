package main

import (
        "context"
        "io"
)

type Item struct {
        Size int64 `json:"size"`
        Children map[string]*Item `json:"children"`
}

type Backend interface {
        List(path string) (*Item, error)
        Get(ctx context.Context, path string, offset, length int64) (*Item, io.Reader, error)
}
