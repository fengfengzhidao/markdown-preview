package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

// Save pasted images next to their document and return a portable relative path.
func (s *fileStore) saveImage(id, dataURL string) (string, error) {
	_, encoded, ok := strings.Cut(dataURL, ";base64,")
	if !ok || !strings.HasPrefix(dataURL, "data:image/") {
		return "", fmt.Errorf("剪贴板中的图片格式无效")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("无法读取剪贴板图片")
	}
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("请粘贴 PNG、JPEG 或 GIF 图片")
	}
	extension := map[string]string{"png": ".png", "jpeg": ".jpg", "gif": ".gif"}[format]
	s.mu.Lock()
	defer s.mu.Unlock()
	path, ok := s.paths[id]
	if !ok {
		return "", fmt.Errorf("请先保存文档，再粘贴图片")
	}
	folder := filepath.Join(filepath.Dir(path), strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+".assets")
	if err := os.MkdirAll(folder, 0755); err != nil {
		return "", fmt.Errorf("无法创建图片目录，%w", err)
	}
	file, err := os.CreateTemp(folder, "image-*"+extension)
	if err != nil {
		return "", fmt.Errorf("无法保存图片，%w", err)
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(file.Name())
		}
	}()
	if _, err := file.Write(data); err != nil {
		return "", fmt.Errorf("图片写入失败，%w", err)
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("图片写入失败，%w", err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("图片写入失败，%w", err)
	}
	relative, err := filepath.Rel(filepath.Dir(path), file.Name())
	if err != nil {
		return "", err
	}
	complete = true
	return filepath.ToSlash(relative), nil
}
