// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (C) 2026 Qjrxy contributors

package server

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"

	"qjrxy/internal/events"
	"qjrxy/internal/store"
)

// 照片池限制：单张 5MB（service 校验）、单次 10 张、每账号 10 张。
// 单次请求体上限留出 multipart 开销，超过直接拒绝，避免一次打满磁盘。
const (
	maxPoolPhotos     = 10
	maxPoolUpload     = 10
	maxPoolUploadBody = 60 << 20
)

type accountPhotoView struct {
	ID        int64  `json:"id"`
	URL       string `json:"url"`
	CreatedAt string `json:"createdAt"`
}

type photoUploadFailure struct {
	Name  string `json:"name"`
	Error string `json:"error"`
}

func accountPhotoURL(accountID, photoID int64) string {
	return "/api/accounts/" + itoa(accountID) + "/photos/" + itoa(photoID)
}

func toAccountPhotoView(accountID int64, p *store.AccountPhoto) accountPhotoView {
	return accountPhotoView{
		ID:        p.ID,
		URL:       accountPhotoURL(accountID, p.ID),
		CreatedAt: p.CreatedAt,
	}
}

// handleListAccountPhotos 返回账号照片池（上传顺序）。
func (s *Server) handleListAccountPhotos(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.svc.DB().AccountByID(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	photos, err := s.svc.DB().ListAccountPhotos(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	views := make([]accountPhotoView, 0, len(photos))
	for i := range photos {
		views = append(views, toAccountPhotoView(id, &photos[i]))
	}
	writeData(w, http.StatusOK, map[string]any{"photos": views})
}

// handleUploadAccountPhotos 接收多张照片进池（multipart 字段 photos 可重复）。
func (s *Server) handleUploadAccountPhotos(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	account, err := s.svc.DB().AccountByID(r.Context(), user.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPoolUploadBody)
	if err := r.ParseMultipartForm(16 << 20); err != nil {
		writeError(w, badRequest("照片上传失败："+err.Error()))
		return
	}
	files := r.MultipartForm.File["photos"]
	if len(files) == 0 {
		writeError(w, badRequest("未找到照片文件"))
		return
	}
	if len(files) > maxPoolUpload {
		writeError(w, badRequest("单次最多上传 10 张照片"))
		return
	}

	count, err := s.svc.DB().CountAccountPhotos(r.Context(), user.ID, account.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	created := make([]accountPhotoView, 0, len(files))
	failed := make([]photoUploadFailure, 0)
	for _, header := range files {
		if count >= maxPoolPhotos {
			failed = append(failed, photoUploadFailure{Name: header.Filename, Error: "照片池最多 10 张"})
			continue
		}
		file, err := header.Open()
		if err != nil {
			failed = append(failed, photoUploadFailure{Name: header.Filename, Error: "读取照片失败"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(file, 6<<20))
		file.Close()
		if err != nil {
			failed = append(failed, photoUploadFailure{Name: header.Filename, Error: "读取照片失败"})
			continue
		}
		rel, err := s.svc.SavePoolPhoto(r.Context(), user.ID, account.ID, header.Filename, data)
		if err != nil {
			failed = append(failed, photoUploadFailure{Name: header.Filename, Error: err.Error()})
			continue
		}
		photo := &store.AccountPhoto{UserID: user.ID, AccountID: account.ID, Path: rel}
		// 每张都在带写锁的事务里「数一张、插一张」，并发上传也突破不了上限。
		if err := s.svc.DB().CreateAccountPhotoWithinLimit(r.Context(), photo, maxPoolPhotos); err != nil {
			_ = os.Remove(s.svc.PhotoAbsPath(rel))
			msg := "保存照片记录失败"
			if errors.Is(err, store.ErrPoolFull) {
				msg = "照片池最多 10 张"
			}
			failed = append(failed, photoUploadFailure{Name: header.Filename, Error: msg})
			continue
		}
		created = append(created, toAccountPhotoView(account.ID, photo))
		count++
	}
	if len(created) > 0 {
		s.svc.Events().Publish(user.ID, events.TopicAccounts)
	}
	writeData(w, http.StatusOK, map[string]any{"photos": created, "failed": failed})
}

// handleDeleteAccountPhoto 删除池内一张照片及其文件。
func (s *Server) handleDeleteAccountPhoto(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	photoID, err := pathPhotoID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.svc.DB().AccountByID(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	rel, err := s.svc.DB().DeleteAccountPhoto(r.Context(), user.ID, id, photoID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, notFound("照片不存在"))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	_ = os.Remove(s.svc.PhotoAbsPath(rel))
	s.svc.Events().Publish(user.ID, events.TopicAccounts)
	writeData(w, http.StatusOK, map[string]any{"message": "照片已删除"})
}

// handleGetAccountPhoto 输出池内照片（支持 ?w= 缩略图）。
func (s *Server) handleGetAccountPhoto(w http.ResponseWriter, r *http.Request) {
	user, err := requireUser(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	photoID, err := pathPhotoID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if _, err := s.svc.DB().AccountByID(r.Context(), user.ID, id); err != nil {
		writeError(w, err)
		return
	}
	photo, err := s.svc.DB().AccountPhotoByID(r.Context(), user.ID, id, photoID)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, notFound("照片不存在"))
		return
	}
	if err != nil {
		writeError(w, err)
		return
	}
	path := s.svc.PhotoAbsPath(photo.Path)
	if _, err := os.Stat(path); err != nil {
		writeError(w, notFound("照片文件不存在"))
		return
	}
	s.servePhotoFile(w, r, path)
}

func pathPhotoID(r *http.Request) (int64, error) {
	raw := r.PathValue("photoId")
	photoID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || photoID <= 0 {
		return 0, badRequest("照片 id 不合法")
	}
	return photoID, nil
}
