package aws

import (
	"crypto/md5"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
)

type InitiateMultipartUpload struct {
	XMLName  xml.Name `xml:"InitiateMultipartUploadResult"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	UploadID string   `xml:"UploadId"`
}

type Part struct {
	XMLName    xml.Name `xml:"Part"`
	PartNumber string   `xml:"PartNumber"`
	ETag       string   `xml:"ETag"`
}

type CompleteMultipartUpload struct {
	XMLName xml.Name `xml:"CompleteMultipartUpload"`
	Parts   []Part   `xml:"Part"`
}

type CompleteMultipartUploadResult struct {
	XMLName  xml.Name `xml:"CompleteMultipartUploadResult"`
	Location string   `xml:"Location"`
	Bucket   string   `xml:"Bucket"`
	Key      string   `xml:"Key"`
	ETag     string   `xml:"ETag"`
}

func (s *S3Server) processUploadPart(w http.ResponseWriter, r *http.Request) {
	contentMD5 := r.Header.Get("content-md5")
	contentLength := r.Header.Get("content-length")
	uploadID := r.URL.Query().Get("uploadId")
	partNumber := r.URL.Query().Get("partNumber")
	log.Printf("[ Multipart Upload ] Received part %s, %s bytes for ID %s\n", partNumber, contentLength, uploadID)
	if s.Verbose {
		log.Printf("[ Multipart Upload ] MD5: %s\n", contentMD5)
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	filename := fmt.Sprintf("%s.%s.part", uploadID, partNumber)
	if ioutil.WriteFile(filename, body, 0644) != nil {
		log.Printf("Error writting body: %v", err)
		return
	}
	w.Header().Set("Etag", contentMD5)
	w.WriteHeader(http.StatusOK)

}

func (s *S3Server) processMultipartInitiate(w http.ResponseWriter, r *http.Request) {
	bucket, filename := ParseS3Url(r.URL)
	log.Printf("[ Multipart Initiate ] %s:%s\n", bucket, filename)
	uploadID := genUploadID()
	log.Printf("[ Multipart Initiate ] UploadID is %s\n", uploadID)
	response := InitiateMultipartUpload{
		Bucket:   bucket,
		Key:      filename,
		UploadID: uploadID,
	}
	xmlString, err := xml.MarshalIndent(response, "", " ")
	if err != nil {
		log.Fatalf("xml.MarshalIndent failed with '%s'\n", err)
	}
	w.WriteHeader(200)
	fmt.Fprintf(w, string(xmlString))
}

func (s *S3Server) processMultipartComplete(w http.ResponseWriter, r *http.Request) {
	uploadID := r.URL.Query().Get("uploadId")
	bucket, filename := ParseS3Url(r.URL)
	log.Printf("[ Multipart Complete ] Folder: %s\tFile:%s\n", bucket, filename)
	log.Printf("[ Multipart Complete ] UploadID is %s\n", uploadID)

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "can't read body", http.StatusBadRequest)
		return
	}
	var mpUpload CompleteMultipartUpload
	xml.Unmarshal(body, &mpUpload)

	hash := md5.New()
	for _, item := range mpUpload.Parts {
		partfname := fmt.Sprintf("%s.%s.part", uploadID, item.PartNumber)
		data, err := ioutil.ReadFile(partfname)
		if err != nil {
			log.Println(err)
		}
		io.WriteString(hash, string(data))

		e := os.Remove(partfname)
		if e != nil {
			log.Fatal(e)
		}

		if err := os.MkdirAll(bucket, os.ModePerm); err != nil {
			log.Printf("[ Multipart Complete ] Error: %s\n", err)
		}

		file, err := os.OpenFile(bucket+"/"+filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Println(err)
		}
		defer file.Close()
		if _, err := file.Write(data); err != nil {
			log.Fatal(err)
		}
	}
	log.Printf("[ Multipart Complete ] Done joining pieces\n")
	response := CompleteMultipartUploadResult{
		Location: "https://" + s.Domain + "/" + bucket + "/" + filename,
		Bucket:   bucket,
		Key:      filename,
		ETag:     string(hash.Sum(nil)),
	}
	xmlString, err := xml.MarshalIndent(response, "", " ")
	if err != nil {
		log.Fatalf("xml.MarshalIndent failed with '%s'\n", err)
	}
	w.WriteHeader(http.StatusOk)
	fmt.Fprintf(w, string(xmlString))
}
