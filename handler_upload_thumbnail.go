package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	//"encoding/base64"
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

	maxMemory := 10 << 20
	err = r.ParseMultipartForm(int64(maxMemory))
	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to retrieve thumbnail", err)
		return
	}
	mediaType := fileHeader.Header.Get("Content-Type")
	mediaTypeSplit := strings.Split(mediaType, "/") //file ext [1]
	if mediaTypeSplit[0] != "image" {
		respondWithError(w, http.StatusBadRequest, "File provided is not an image", fmt.Errorf("not an image"))
	}
	fmt.Printf("File type: %s\n", mediaTypeSplit[1])
	//imageData, err := io.ReadAll(file)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to read image data", err)
	}
	dbVideo, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to retrieve video", err)
	}
	randID := make([]byte, 32)
	rand.Read(randID)
	encoded := base64.RawURLEncoding.EncodeToString(randID)
	filepath := path.Join(cfg.filepathRoot, "assets", encoded + "." + mediaTypeSplit[1])
	fmt.Printf("filepath: %s\n", filepath)
	fileOut, err := os.Create(filepath)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to create file", fmt.Errorf("failed to create file"))
	}
	io.Copy(fileOut, file)
	//thumb := thumbnail{imageData, mediaType}
	//encodedThumb := base64.StdEncoding.EncodeToString(imageData)
	//dataUrl := "data:" + mediaType + ";base64," + encodedThumb
	//
	//videoThumbnails[videoID] = thumb
	thumbUrl := "http://localhost:" + cfg.port + "/app/assets/" + encoded + "." + mediaTypeSplit[1]
	dbVideo.ThumbnailURL = &thumbUrl
	err = cfg.db.UpdateVideo(dbVideo)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to save updated video to database", err)
	}
	respondWithJSON(w, http.StatusOK, dbVideo)
}
