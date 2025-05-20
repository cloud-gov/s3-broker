package s3

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	brokertags "github.com/cloud-gov/go-broker-tags"
	"github.com/cloud-gov/s3-broker/cmd/tasks/tags"
	"golang.org/x/text/message/catalog"
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
			found = true
			break
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

func ReconcileS3BucketTags(catalog *catalog.Catalog, db *gorm.DB, s3Client s3iface.S3API, tagManager brokertags.TagManager) error {
	rows, err := db.Model(&s3bucket.S3BucketInstance{}).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var bucketInstance s3bucket.S3BucketInstance
		db.ScanRows(rows, &bucketInstance)

		plan, _ := catalog.ElasticsearchService.FetchPlan(bucketInstance.PlanID)
		if plan.Name == "" {
			return fmt.Errorf("error getting plan %s for bucket %s", bucketInstance.PlanID, bucketInstance.BucketName)
		}

		generatedTags, err := tags.GenerateTags(
			tagManager,
			catalog.S3Service.Name,
			plan.Name,
			brokertags.ResourceGUIDs{
				InstanceGUID:     bucketInstance.Uuid,
				SpaceGUID:        bucketInstance.SpaceGUID,
				OrganizationGUID: bucketInstance.OrganizationGUID,
			},
		)
		if err != nil {
			return fmt.Errorf("error generating new tags for bucket %s: %s", &bucketInstance.BucketName, err)
		}

		S3Tags := s3bucket.ConvertTagsToS3Tags(generatedTags)
		err = processS3Bucket(S3Client, bucketInstance.BucketName, S3Tags)
		if err != nil {
			return err
		}
	}

	return nil
}
