package main

import (
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cloudfoundry-community/go-cfenv"
)

const (
	key   = "test-key"
	value = "test-value"
)

var creds map[string]interface{}

func main() {
	env, _ := cfenv.Current()
	services, _ := env.Services.WithLabel(os.Getenv("SERVICE_NAME"))
	if len(services) != 1 {
		log.Fatalf("Expected one service instance; got %d", len(services))
	}
	creds = services[0].Credentials

	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+os.Getenv("PORT"), nil)
}

func handler(w http.ResponseWriter, r *http.Request) {
	config := &aws.Config{
		Region: aws.String(creds["region"].(string)),
		Credentials: credentials.NewStaticCredentials(
			creds["username"].(string),
			creds["password"].(string),
			"",
		),
	}

	sess, err := session.NewSession(config)
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	svc := s3.New(sess)

	// Test put object
	_, err = svc.PutObject(&s3.PutObjectInput{
		Body:   strings.NewReader(value),
		Bucket: aws.String(creds["name"].(string)),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Test get object
	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(creds["name"].(string)),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Test object contents
	body, err := ioutil.ReadAll(result.Body)
	if string(body) != value {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Test delete object
	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(creds["name"].(string)),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
