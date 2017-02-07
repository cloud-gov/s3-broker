package main

import (
	"fmt"
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
			creds["access_key_id"].(string),
			creds["secret_access_key"].(string),
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
	if os.Getenv("IS_ENCRYPTED") == "true" {
		// Unencrypted PUT should be rejected
		_, err = svc.PutObject(&s3.PutObjectInput{
			Body:   strings.NewReader(value),
			Bucket: aws.String(creds["bucket"].(string)),
			Key:    aws.String(key),
		})
		if err == nil {
			log.Println("Unencrypted PUT was not rejected")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Encrypted PUT should be accepted
		_, err = svc.PutObject(&s3.PutObjectInput{
			Body:                 strings.NewReader(value),
			Bucket:               aws.String(creds["bucket"].(string)),
			Key:                  aws.String(key),
			ServerSideEncryption: aws.String("AES256"),
		})
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		// Any PUT should be accepted
		_, err = svc.PutObject(&s3.PutObjectInput{
			Body:   strings.NewReader(value),
			Bucket: aws.String(creds["bucket"].(string)),
			Key:    aws.String(key),
		})
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Test get object
	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(creds["bucket"].(string)),
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

	// Test public access
	resp, _ := http.Get(
		fmt.Sprintf(
			"https://s3-%s.amazonaws.com/%s/%s",
			creds["region"].(string),
			creds["bucket"].(string),
			key,
		),
	)
	expectedCode := http.StatusForbidden
	if os.Getenv("IS_PUBLIC") == "true" {
		expectedCode = http.StatusOK
	}
	if resp.StatusCode != expectedCode {
		log.Println(fmt.Sprintf("expected code %d; got %d", expectedCode, resp.StatusCode))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Test delete object
	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(creds["bucket"].(string)),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
