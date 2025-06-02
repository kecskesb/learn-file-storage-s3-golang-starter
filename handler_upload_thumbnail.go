package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse form", err)
		return
	}
	data, headers, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get thumbnail file", err)
		return
	}
	defer data.Close()
	mediaType := headers.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Content-Type header is required", nil)
		return
	}

	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Unsupported media type", nil)
		return
	}

	imageData, err := io.ReadAll(data)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't read thumbnail data", err)
		return
	}
	videoMeta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata", err)
		return
	}
	if videoMeta.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You are not allowed to upload a thumbnail for this video", nil)
		return
	}
	newThumbnail := thumbnail{
		data:      imageData,
		mediaType: mediaType,
	}
	videoThumbnails[videoID] = newThumbnail

	//newThumbnailUrl := fmt.Sprintf("http://localhost:8091/api/thumbnails/%s", videoID.String())
	//videoMeta.ThumbnailURL = &newThumbnailUrl
	//err = cfg.db.UpdateVideo(videoMeta)
	//if err != nil {
	//	respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata with thumbnail URL", err)
	//	return
	//}

	//imageBase64 := base64.StdEncoding.EncodeToString(imageData)
	//dataUrl := fmt.Sprintf("data:%s;base64,%s", mediaType, imageBase64)
	//videoMeta.ThumbnailURL = &dataUrl
	//err = cfg.db.UpdateVideo(videoMeta)
	//if err != nil {
	//	respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata with data URL", err)
	//	return
	//}

	fileExtension := mediaType[strings.Index(mediaType, "/")+1:]
	byteSlice := make([]byte, 32)
	_, err = rand.Read(byteSlice)
	fileName := base64.RawURLEncoding.EncodeToString(byteSlice)
	filePath := filepath.Join(cfg.assetsRoot, fmt.Sprintf("%s.%s", fileName, fileExtension))
	fileHandle, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create thumbnail file", err)
		return
	}
	_, err = io.Copy(fileHandle, bytes.NewReader(imageData))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't write thumbnail data to file", err)
		return
	}

	newThumbnailUrl := fmt.Sprintf("http://localhost:8091/%s", filePath)
	videoMeta.ThumbnailURL = &newThumbnailUrl
	err = cfg.db.UpdateVideo(videoMeta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata with thumbnail URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMeta)
}
