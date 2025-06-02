package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {

	http.MaxBytesReader(w, r.Body, 10<<30)

	videoIDRaw := r.PathValue("videoID")
	if videoIDRaw == "" {
		respondWithError(w, http.StatusBadRequest, "Video ID is required", nil)
		return
	}

	videoID, err := uuid.Parse(videoIDRaw)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid Video ID format", err)
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
	videoMeta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video metadata", err)
		return
	}

	if videoMeta.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You are not allowed to upload this video", nil)
		return
	}

	file, headers, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get video file", err)
		return
	}
	defer func() { _ = file.Close() }()

	mediaType := headers.Header.Get("Content-Type")
	if mediaType == "" {
		respondWithError(w, http.StatusBadRequest, "Content-Type header is required", nil)
		return
	}
	if mediaType != "video/mp4" {
		respondWithError(w, http.StatusUnsupportedMediaType, "Unsupported media type", nil)
		return
	}

	tempFile, err := os.CreateTemp("", "tubely-video-*.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temporary file", err)
		return
	}
	defer func() { _ = os.Remove(tempFile.Name()) }()
	defer func() { _ = tempFile.Close() }()
	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save video file", err)
		return
	}

	randomBytes := make([]byte, 32)
	_, _ = rand.Read(randomBytes)
	fileName := base64.RawURLEncoding.EncodeToString(randomBytes) + ".mp4"
	_, _ = tempFile.Seek(0, io.SeekStart)
	_, err = cfg.s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      &cfg.s3Bucket,
		Key:         &fileName,
		Body:        tempFile,
		ContentType: &mediaType,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video to S3", err)
		return
	}

	s3Url := fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, fileName)
	videoMeta.VideoURL = &s3Url
	err = cfg.db.UpdateVideo(videoMeta)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video metadata with video URL", err)
		return
	}

	respondWithJSON(w, http.StatusOK, videoMeta)

}
