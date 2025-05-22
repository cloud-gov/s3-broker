package s3

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	brokertags "github.com/cloud-gov/go-broker-tags"
	task_tag "github.com/cloud-gov/s3-broker/cmd/tasks/tags"
	cf "github.com/cloudfoundry/go-cfclient/v3/client"
)

func getS3BucketTags(s3Client s3iface.S3API, bucketName string) ([]*s3.Tag, error) {
	response, err := s3Client.GetBucketTagging(&s3.GetBucketTaggingInput{
		Bucket: aws.String(bucketName),
	})

	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "NoSuchTagSet" {
			return []*s3.Tag{}, nil
		}
		return nil, fmt.Errorf("could not get tags for bucket %s: %s", bucketName, err)
	}
	return response.TagSet, nil
}

func doS3BucketTagsContainGeneratedTags(existingTags []*s3.Tag, generatedTags []*s3.Tag) bool {
	for _, v := range generatedTags {
		if *v.Key == "Created at" || *v.Key == "Updated at" {
			continue
		}
		found := false
		for _, t := range existingTags {
			if *v.Key == *t.Key && *v.Value == *t.Value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func processS3Bucket(s3Client s3iface.S3API, bucketName string, generatedTags []*s3.Tag) error {
	existingTags, err := getS3BucketTags(s3Client, bucketName)
	if err != nil {
		return err
	}

	if doS3BucketTagsContainGeneratedTags(existingTags, generatedTags) {
		log.Printf("tags already up to date for bucket %s", bucketName)
		return nil
	}

	log.Printf("updating tags for resource %s", bucketName)
	_, err = s3Client.PutBucketTagging(&s3.PutBucketTaggingInput{
		Bucket: aws.String(bucketName),
		Tagging: &s3.Tagging{
			TagSet: generatedTags,
		},
	})
	if err != nil {
		return fmt.Errorf("error adding new tags for bucket %s: %s", bucketName, err)
	}

	log.Printf("finished updating tags for bucket %s", bucketName)
	return nil
}

func convertTagsToS3Tags(tags map[string]string) []*s3.Tag {
	var s3Tags []*s3.Tag
	for k, v := range tags {
		tag := s3.Tag{
			Key:   aws.String(k),
			Value: aws.String(v),
		}

		s3Tags = append(s3Tags, &tag)
	}
	return s3Tags
}

func ReconcileS3BucketTags(s3Client s3iface.S3API, tagManager brokertags.TagManager, cfClient *cf.Client, String Environment) error {
	log.Println(Environment)
	output, err := s3Client.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		return fmt.Errorf("error listing buckets: %w", err)
	}

	for _, bucket := range output.Buckets {
		if bucket == nil || bucket.Name == nil {
			log.Println("This bucket is empty %s", bucket)
			continue
		}
		bucketName := *bucket.Name

		if !strings.HasPrefix(bucketName, Environment + "cg-") {
			continue
		}

		instanceUUID := strings.TrimPrefix(bucketName, Environment + "cg-")

		if Environment == ""{
			continue
		}
		if 1 == 1 {
			continue
		}

		taggingOutput, err := s3Client.GetBucketTagging(&s3.GetBucketTaggingInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "NoSuchTagSet" {
				log.Printf("no tags found for bucket %s, skipping", bucketName)
			}
			log.Printf("error getting tags for: %s: %s", bucketName, err)
			continue
		}

		tags := make(map[string]string)
		for _, tag := range taggingOutput.TagSet {
			tags[*tag.Key] = *tag.Value
		}

		instance, err := cfClient.ServiceInstances.Get(context.Background(), instanceUUID)
		if err != nil {
			log.Printf("Could not find service instance for GUID %s", instanceUUID)
			continue
		}
		planGUID := instance.Relationships.ServicePlan.Data.GUID

		plan, err := cfClient.ServicePlans.Get(context.Background(), planGUID)
		if err != nil {
			log.Printf("Could not find service plan for instance %s", instanceUUID)
			continue
		}

		generatedTags, err := task_tag.GenerateTags(
			tagManager,
			"S3",
			plan.Name,
			brokertags.ResourceGUIDs{
				InstanceGUID: instanceUUID,
			},
		)
		if err != nil {
			return fmt.Errorf("error generating new tags for bucket %s: %s", bucketName, err)
		}

		s3Tags := convertTagsToS3Tags(generatedTags)
		err = processS3Bucket(s3Client, bucketName, s3Tags)
		if err != nil {
			return err
		}
	}

	return nil
}
