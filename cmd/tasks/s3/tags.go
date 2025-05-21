package s3

import (
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	brokertags "github.com/cloud-gov/go-broker-tags"
	"github.com/cloud-gov/s3-broker/broker"
	s3bucket "github.com/cloud-gov/s3-broker/cmd/tasks/s3"
	task_tag "github.com/cloud-gov/s3-broker/cmd/tasks/tags"
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

func ReconcileS3BucketTags(s3Client s3iface.S3API, tagManager brokertags.TagManager) error {
	output, err := s3Client.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		return fmt.Errorf("error listing buckets: %w", err)
	}

	for _, bucket := range output.Buckets {
		if bucket == nil || bucket.Name == nil {
			continue
		}
		bucketName := *bucket.Name
		if !strings.HasPrefix(bucketName, "cg-") {
			continue
		}
		instanceUUID := strings.TrimPrefix(bucketName, "cg-")
		taggingOutput, err := s3Client.GetBucketTagging(&s3.GetBucketTaggingInput{
			Bucket: aws.String(bucketName),
		})
		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == "NoSuchTagSet" {
				log.Printf("No tags found for bucket %s, skipping", &bucketName)
			}
			log.Printf(" Error getting tags for: %s: %s", &bucketName, err)
			continue
		}

		tags := make(map[string]string)
		for _, tag := range taggingOutput.TagSet {
			tags[*tag.Key] = *tag.Value
		}

		planID, ok := tags["Plan ID"]
		if !ok {
			log.Printf(" Plan ID not found for %s", bucketName)
			continue
		}
		spaceID, ok := tags["Space ID"]
		if !ok {
			log.Printf(" Space ID not found for %s", bucketName)
			continue
		}
		organizationID, ok := tags["Organization ID"]
		if !ok {
			log.Printf(" Organization ID not found for %s", bucketName)
			continue
		}

		plan, found := broker.FindServicePlan(planID)
		if !found {
			log.Printf("error getting plan %s for bucket %s", planID, bucketName)
			continue
		}

		generatedTags, err := task_tag.GenerateTags(
			tagManager,
			"S3",
			plan,
			brokertags.ResourceGUIDs{
				InstanceGUID:     instanceUUID,
				SpaceGUID:        spaceID,
				OrganizationGUID: organizationID,
			},
		)
		if err != nil {
			return fmt.Errorf("error generating new tags for bucket %s: %s", bucketName, err)
		}

		S3Tags := s3bucket.ConvertTagsToS3Tags(generatedTags)
		err = processS3Bucket(s3Client, bucketName, S3Tags)
		if err != nil {
			return err
		}
	}

	return nil
}
