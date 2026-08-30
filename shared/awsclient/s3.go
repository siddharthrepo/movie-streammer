package awsclient

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

func NewS3(region, endpoint, accessKey, secretKey string, usePathStyle bool) *s3.Client {
	opts := s3.Options{
		Region:       region,
		UsePathStyle: usePathStyle,
		Credentials: credentials.NewStaticCredentialsProvider(
			accessKey, secretKey, "",
		),
	}
	if endpoint != "" {
		opts.BaseEndpoint = aws.String(endpoint)
	}
	return s3.New(opts)
}

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var nsk *types.NoSuchKey
	var nsb *types.NoSuchBucket
	var nf *types.NotFound
	if errors.As(err, &nsk) || errors.As(err, &nsb) || errors.As(err, &nf) {
		return true
	}
	var ae smithy.APIError
	if errors.As(err, &ae) {
		switch ae.ErrorCode() {
		case "NotFound", "NoSuchKey", "NoSuchBucket", "NoSuchUpload", "404":
			return true
		}
	}
	return false
}

func IsAlreadyOwned(err error) bool {
	var ae smithy.APIError
	if errors.As(err, &ae) {
		code := ae.ErrorCode()
		return code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists"
	}
	return false
}
