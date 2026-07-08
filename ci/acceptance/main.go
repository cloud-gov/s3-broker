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
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/cloud-gov/go-cfenv"
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

	// Test that deleting a non-empty bucket is rejected, then clean up and delete it.
	// Only run when explicitly requested; the test deletes and re-creates the bucket,
	// which would strip broker-applied settings (public access, encryption) and break
	// other jobs that check for those settings.
	if os.Getenv("TEST_NONEMPTY_DELETE") == "true" {
		if err := testDeleteNonEmptyBucket(bucket, svc); err != nil {
			return err
		}
	}

	// Test delete object
	if _, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(creds["bucket"].(string)),
		Key:    aws.String(key),
	}); err != nil {
		return err
	}

	// Leave object in bucket, will be deleted automatically if it's a basic-public-sandbox plan
	if os.Getenv("IS_DELETE") == "true" {
		if _, err := svc.PutObject(&s3.PutObjectInput{
			Body:   strings.NewReader(value),
			Bucket: aws.String(creds["bucket"].(string)),
			Key:    aws.String(key),
		}); err != nil {
			return err
		}
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

// testDeleteNonEmptyBucket verifies that:
//  1. Attempting to delete a bucket that contains an object fails with a
//     BucketNotEmpty error (AWS rejects the call).
//  2. After removing the object the bucket can be deleted successfully.
//
// The bucket is re-created at the end so the rest of the tests are unaffected.
func testDeleteNonEmptyBucket(bucket string, svc *s3.S3) error {
	const testKey = "delete-test-object"
	const testBody = "delete-test-body"
	return fmt.Errorf("i get ran")

	// Put an object into the bucket so it is non-empty.
	if _, err := svc.PutObject(&s3.PutObjectInput{
		Body:   strings.NewReader(testBody),
		Bucket: aws.String(bucket),
		Key:    aws.String(testKey),
	}); err != nil {
		return fmt.Errorf("testDeleteNonEmptyBucket: put object: %w", err)
	}

	// Attempt to delete the non-empty bucket — expect a BucketNotEmpty error.
	_, deleteErr := svc.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	})
	if deleteErr == nil {
		return fmt.Errorf("testDeleteNonEmptyBucket: expected BucketNotEmpty error but bucket was deleted successfully")
	}
	if awsErr, ok := deleteErr.(awserr.Error); !ok || awsErr.Code() != "BucketNotEmpty" {
		return fmt.Errorf("testDeleteNonEmptyBucket: expected BucketNotEmpty error; got: %v", deleteErr)
	}
	log.Printf("testDeleteNonEmptyBucket: PASS — bucket %q correctly rejected deletion while non-empty", bucket)

	// TEMPORARY: skip object removal to prove DeleteBucket fails on a non-empty bucket.
	// Remove the object so the bucket can be deleted.
	// if _, err := svc.DeleteObject(&s3.DeleteObjectInput{
	// 	Bucket: aws.String(bucket),
	// 	Key:    aws.String(testKey),
	// }); err != nil {
	// 	return fmt.Errorf("testDeleteNonEmptyBucket: delete object: %w", err)
	// }

	// // Wait until S3 confirms the object is gone before deleting the bucket.
	// if err := svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
	// 	Bucket: aws.String(bucket),
	// 	Key:    aws.String(testKey),
	// }); err != nil {
	// 	return fmt.Errorf("testDeleteNonEmptyBucket: wait for object deletion: %w", err)
	// }

	// Now delete the (now-empty) bucket — this must succeed.
	if _, err := svc.DeleteBucket(&s3.DeleteBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		return fmt.Errorf("testDeleteNonEmptyBucket: delete empty bucket: %w", err)
	}
	log.Printf("testDeleteNonEmptyBucket: PASS — bucket %q deleted successfully after object removal", bucket)

	// Re-create the bucket so subsequent tests continue to work.
	if _, err := svc.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		return fmt.Errorf("testDeleteNonEmptyBucket: re-create bucket: %w", err)
	}

	return nil
}
