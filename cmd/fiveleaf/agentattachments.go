package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"fiveleaf/internal/remote"
	"fiveleaf/locales"
	"github.com/egoist/mygo/ui"
)

const chatPreviewSide = 480

func decodeChatImage(data []byte, ext string) ui.ImageSource {
	return decodeSizedImage(data, ext, chatPreviewSide, 4096)
}

func isChatImage(path string) bool {
	return filepath.IsAbs(path) && !remote.Is(path) && (isImage(path) || isIconFile(path))
}

func chatTextAttachment(name, path, body string) chatDraftRef {
	ref := chatDraftRef{chatDetailSource: chatDetailSource{label: filepath.Base(name), path: path}, file: name,
		content: agentContextText(name, 0, 0, body), bytes: int64(len(body))}
	ref.previewText = body
	return ref
}

func readChatAttachment(root, path string) (chatDraftRef, error) {
	ref := chatDraftRef{chatDetailSource: chatDetailSource{label: filepath.Base(path), path: path}, file: path, content: path + "\n"}
	if isChatImage(path) {
		info, err := os.Stat(path)
		if err != nil {
			return ref, err
		}
		if !info.Mode().IsRegular() {
			return ref, errors.New(locales.ErrImageNotRegular())
		}
		ref.bytes = info.Size()
		if data, err := readImage(path); err == nil {
			ref.preview = decodeChatImage(data, filepath.Ext(path))
		}
		return ref, nil
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ref, err
	}
	ext := strings.ToLower(filepath.Ext(path))
	limit := int64(agentTextLimit)
	if chatDocumentType(ext) {
		limit = 16 << 20
	}
	data, size, err := readAgentFileData(root, canonical, limit)
	ref.bytes = size
	tooLarge := errors.Is(err, errAgentFileTooLarge)
	if err != nil && !tooLarge {
		return ref, err
	}
	if chatDocumentType(ext) || isBinary(data) || !tooLarge && !utf8.Valid(data) {
		if !tooLarge && chatDocumentType(ext) {
			ref.preview = chatDocumentThumbnail(data, ext)
		}
		if ref.preview == nil {
			ref.previewText = chatBinaryExcerpt(data)
		}
		return ref, nil
	}
	if tooLarge {
		return ref, err
	}
	return chatTextAttachment(path, path, string(data)), nil
}

func chatDocumentType(ext string) bool {
	switch ext {
	case ".pdf", ".docx", ".pptx", ".xlsx", ".pages", ".key", ".numbers":
		return true
	}
	return false
}

func chatDocumentThumbnail(data []byte, ext string) ui.ImageSource {
	var poppler string
	if ext == ".pdf" {
		poppler, _ = exec.LookPath("pdftoppm")
	}
	if poppler == "" && runtime.GOOS != "darwin" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "fiveleaf-document-preview-*")
	if err != nil {
		return nil
	}
	defer os.RemoveAll(dir)
	var thumb string
	side := strconv.Itoa(chatPreviewSide)
	if poppler != "" {
		prefix := filepath.Join(dir, "page")
		command := exec.CommandContext(ctx, poppler, "-f", "1", "-l", "1", "-scale-to", side, "-singlefile", "-png", "-", prefix)
		command.Stdin = bytes.NewReader(data)
		if command.Run() != nil {
			return nil
		}
		thumb = prefix + ".png"
	} else {
		input := filepath.Join(dir, "document"+ext)
		if os.WriteFile(input, data, 0600) != nil || exec.CommandContext(ctx, "/usr/bin/qlmanage", "-t", "-s", side, "-o", dir, input).Run() != nil {
			return nil
		}
		thumb = input + ".png"
	}
	png, err := readImage(thumb)
	if err != nil {
		return nil
	}
	return decodeChatImage(png, ".png")
}

func chatBinaryExcerpt(data []byte) string {
	if bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		if archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data))); err == nil {
			var names strings.Builder
			for index, entry := range archive.File {
				if index > 0 {
					names.WriteByte('\n')
				}
				if names.Len() >= 64<<10 {
					names.WriteString("…")
					break
				}
				names.WriteString(strings.ToValidUTF8(entry.Name, "�"))
			}
			return names.String()
		}
	}
	return strings.TrimSpace(hex.Dump(data[:min(len(data), 64)]))
}

func (a *app) pasteChatAttachment(chat *agentChat, text string) bool {
	if paths := a.host.clipboardFiles(); len(paths) != 0 {
		paths = slices.Clone(paths)
		a.postChatAction(chat, func() { a.addChatDetailFiles(chat, paths) })
		return true
	}
	if text != "" {
		return false
	}
	data := a.host.clipboardImage()
	if len(data) == 0 {
		return false
	}
	a.postChatAction(chat, func() { a.attachClipboardImage(chat, data) })
	return true
}

func (a *app) attachClipboardImage(chat *agentChat, data []byte) {
	if len(data) > maxOpenBytes {
		chat.err = locales.ErrImageTooLarge()
		return
	}
	a.loadChatAttachment(chat, func() (chatDraftRef, error) {
		preview := decodeChatImage(data, ".png")
		if preview == nil {
			return chatDraftRef{}, errors.New(locales.ErrNotImage())
		}
		file, err := os.CreateTemp("", "fiveleaf-image-*.png")
		if err != nil {
			return chatDraftRef{}, errors.New(locales.FailPasteImage(err.Error()))
		}
		path := file.Name()
		_, writeErr := file.Write(data)
		if err := errors.Join(writeErr, file.Close()); err != nil {
			os.Remove(path)
			return chatDraftRef{}, errors.New(locales.FailPasteImage(err.Error()))
		}
		return chatDraftRef{chatDetailSource: chatDetailSource{label: locales.AgentPastedImage(), path: path, preview: preview},
			file: path, content: path + "\n", temporary: true, bytes: int64(len(data))}, nil
	}, nil)
}
