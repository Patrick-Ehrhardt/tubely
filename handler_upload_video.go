package main

import (
	"net/http"
	"github.com/google/uuid"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	//"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"fmt"
	"mime"
	"os"
	"bytes"
	"os/exec"
	"io"
	"context"
	"encoding/json"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"crypto/rand"
	"encoding/base64"
	"strings"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	//reader := http.MaxBytesReader(w, r.Body, 1 << 30)
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}
	bearer_token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to retrieve bearer token", err)
	}
	userID, err := auth.ValidateJWT(bearer_token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to validate JWT", err)
	}
	videoMeta, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to retrieve video metadata", err)
	}
	if videoMeta.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "Unauthorized user", fmt.Errorf("User ID mismatch"))
	}
	videoFile, _, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Failed to parse video file", err)
	}
	defer videoFile.Close()
	mediaType, _, err := mime.ParseMediaType("video/mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not parse media type", err)
	}
	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Could not create temporary file", err)
	}
	defer os.Remove("tubely-upload.mp4")
	defer tempFile.Close()
	written, err := io.Copy(tempFile, videoFile)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error writing to temporary file", err)
	}
	fmt.Printf("Data written: %d", written)
	tempFile.Seek(0, io.SeekStart)
	randID := make([]byte, 32)
	rand.Read(randID)
	encoded := base64.RawURLEncoding.EncodeToString(randID)
	mediaTypeSplit := strings.Split(mediaType, "/")
	bucketKey := encoded + "." + mediaTypeSplit[1]

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error obtaining aspect ratio", err)
	}
	if aspectRatio == "16:9" {
		bucketKey = "landscape/" + bucketKey
	}
	if aspectRatio == "9:16" {
		bucketKey = "portrait/" + bucketKey
	}

	processedFileName, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error processing video", err)
	}
	processedVideo, err := os.Open(processedFileName)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error opening processed video", err)
	}
	defer processedVideo.Close()
	defer os.Remove(processedVideo.Name())

	
	//fmt.Printf("DEBUG\nBucket: %s\nKey: %s\nContentType: %s\n", cfg.s3Bucket, bucketKey, mediaType)
	putParams := &s3.PutObjectInput{
		Bucket: &cfg.s3Bucket,
		Key: &bucketKey,
		Body: processedVideo,
		ContentType: &mediaType,


	}
	_, err = cfg.s3Client.PutObject(context.Background(), putParams)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to put object to S3 bucket", err)
	}
	newVideoUrl := "https://tubely-78957.s3.us-east-2.amazonaws.com/" + bucketKey
	videoMeta.VideoURL = &newVideoUrl
	fmt.Printf("\nDEBUG\nnewVideoUrl: %s\nbucketKey: %s\naspectRatio: %s", newVideoUrl, bucketKey, aspectRatio)
	err = cfg.db.UpdateVideo(videoMeta)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to update video metadata", err)
	}
}

func getVideoAspectRatio (filePath string) (string, error) {
	outBuffer := &bytes.Buffer{}
	/*ffprobeCmd := exec.Cmd{
		Path: "ffprobe",
		Args: []string{"-v", "error", "-print_format", "json", "-show_streams", filePath},
		Stdout: outBuffer,
	}*/
	ffprobeCmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	ffprobeCmd.Stdout = outBuffer
	err := ffprobeCmd.Run()
	if err != nil {
		return "", fmt.Errorf("Failed to run ffprobe command: %s", err)
	}
	type probeOut struct {
		Streams []struct {
			Width int `json:"width"`
			Height int `json:"height"`
		} `json:"streams"`
	}
	pOut := probeOut{}
	err = json.Unmarshal(outBuffer.Bytes(), &pOut)
	if err != nil {
		return "", fmt.Errorf("Error unmarshalling JSON from ffProbe: %s", err)
	}
	if len(pOut.Streams) == 0 {
		return "", fmt.Errorf("No JSON retrieved from ffprobe")
	}
	aspectRatio := pOut.Streams[0].Width /  pOut.Streams[0].Height
	fmt.Printf("DEBUG: aspectRatio: %f", aspectRatio)
	if aspectRatio == 1 {
		return "16:9", nil
	}
	if aspectRatio == 0 {
		return "9:16", nil
	}
	return "other", nil

}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilename := filePath + ".processing"
	cmd := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilename)
	err := cmd.Run()
	if err != nil {
		return "", err
	}
	return outputFilename, nil
}