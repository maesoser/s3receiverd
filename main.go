package main

/*
	This binary serves an S# endpoint for Cloudflare logpush logs to be received.
    "s3:PutObject" ; https://docs.aws.amazon.com/AmazonS3/latest/API/API_PutObject.html
    https://aws.amazon.com/premiumsupport/knowledge-center/s3-multipart-upload-cli/
	https://docs.aws.amazon.com/AmazonS3/latest/dev/mpuoverview.html
	https://docs.aws.amazon.com/AmazonS3/latest/dev/sdksupportformpu.html
	https://docs.aws.amazon.com/AmazonS3/latest/API/API_CreateMultipartUpload.html
*/

import (
	"crypto/tls"
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/maesoser/logrecv/pkg/aws"
	"github.com/maesoser/logrecv/pkg/easycert"
)

func GetEnvStr(name, value string) string {
	if os.Getenv(name) != "" {
		return os.Getenv(name)
	}
	return value
}

func main() {

	log.SetPrefix("[cflogrcv] ")
	log.SetFlags(log.Ltime)
	log.SetOutput(os.Stderr)

	ListenAddr := flag.String("port", GetEnvStr("SERVER_PORT", "0.0.0.0:8443"), "Server Port")
	KeyArg := flag.String("key", GetEnvStr("SERVER_KEY", "data/key.pem"), "Server Key")
	CertArg := flag.String("cert", GetEnvStr("SERVER_CERT", "data/certificate.pem"), "Server Certificate")
	SNIName := flag.String("domain", GetEnvStr("SNI_NAME", "localhost"), "Server Name")
	AccessKey := flag.String("access-key", GetEnvStr("ACCESS_KEY", "AKIAI44QH8DHBEXAMPLE"), "Access Key ID")
	Secret := flag.String("secret", GetEnvStr("SECRET", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"), "Secret access key")
	Verbose := flag.Bool("verbose", false, "Verbose Output")
	Aggregate := flag.Bool("aggregate", false, "Agregate logs on a daily basis")

	flag.Parse()

	server := aws.S3Server{
		Verbose:   *Verbose,
		Domain:    *SNIName,
		AccessKey: *AccessKey,
		Secret:    *Secret,
		Aggregate: *Aggregate,
	}

	http.HandleFunc("/", server.ProcessRequest)
	log.Printf("Starting S3 service on %s ...", *ListenAddr)
	srv := &http.Server{
		Addr:         *ListenAddr,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		TLSConfig: &tls.Config{
			PreferServerCipherSuites: true,
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.X25519,
			},
		},
	}
	if err := srv.ListenAndServeTLS(*CertArg, *KeyArg); err != nil {
		log.Printf("Error: %v\n", err)
		log.Println("Generating self signed certificate")
		bundle, err := easycert.GenerateGeneric()
		if err != nil {
			panic(err)
		}
		log.Println("Certificate generated")
		srv.TLSConfig.Certificates = []tls.Certificate{bundle}
		srv.TLSConfig.ServerName = *SNIName
		if err := srv.ListenAndServeTLS("", ""); err != nil {
			log.Println(err)
		}
	}
}
