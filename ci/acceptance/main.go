package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reflect"
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

	bucket := creds["bucket"].(string)
	additionalBuckets := []string{}
	for _, additionalBucket := range creds["additional_buckets"].([]interface{}) {
		additionalBuckets = append(additionalBuckets, additionalBucket.(string))
	}

	if os.Getenv("ADDITIONAL_INSTANCE_NAME") != "" {
		if len(additionalBuckets) != 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	buckets := append(additionalBuckets, bucket)

	for _, bucket := range buckets {
		if err := testBucket(bucket, svc); err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusOK)
}

func testBucket(bucket string, svc *s3.S3) error {
	// Test put object
	if _, err := svc.PutObject(&s3.PutObjectInput{
		Body:   strings.NewReader(value),
		Bucket: aws.String(creds["bucket"].(string)),
		Key:    aws.String(key),
	}); err != nil {
		return err
	}

	// Test get object
	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(creds["bucket"].(string)),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}

	// Test object contents
	body, err := ioutil.ReadAll(result.Body)
	if err != nil {
		return err
	}
	if string(body) != value {
		return fmt.Errorf("Got value %s; expected %s", string(body), value)
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
		return fmt.Errorf("expected code %d; got %d", expectedCode, resp.StatusCode)
	}

	// Test delete object
	if _, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(creds["bucket"].(string)),
		Key:    aws.String(key),
	}); err != nil {
		return err
	}

	expectedEncryption := os.Getenv("ENCRYPTION")
	if expectedEncryption != "" {
		var expectedConfig s3.ServerSideEncryptionConfiguration
		if err := json.Unmarshal([]byte(expectedEncryption), &expectedConfig); err != nil {
			return err
		}
		encryptionOutput, err := svc.GetBucketEncryption(&s3.GetBucketEncryptionInput{
			Bucket: aws.String(creds["bucket"].(string)),
		})
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(expectedConfig, *encryptionOutput.ServerSideEncryptionConfiguration) {
			return fmt.Errorf("expected encryption config %+v; got %+v", expectedConfig, encryptionOutput.ServerSideEncryptionConfiguration)
		}
	}

	return nil
}
