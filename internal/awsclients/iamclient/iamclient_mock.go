package iamclient

import (
	"context"
	"errors"
	"log"

	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// FakeIamClient implements the necessary IAM API for testing.
type FakeIamClient struct {
	Region string
	// PageOutputs holds the pages to return.
	IamRolesPageOutputs                  []*iam.ListRolesOutput
	ListOpenIDConnectProviderPageOutputs *iam.ListOpenIDConnectProvidersOutput
	// ErrorOnCall simulates an error on the specified call index.
	ErrorOnCall int
	CallCount   int
}

func (f *FakeIamClient) ListRoles(ctx context.Context, input *iam.ListRolesInput, optFns ...func(*iam.Options)) (*iam.ListRolesOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	// If the call count equals ErrorOnCall, simulate an error.
	if f.CallCount == f.ErrorOnCall {
		return nil, errors.New("pagination error")
	}
	if f.CallCount < len(f.IamRolesPageOutputs) {
		log.Printf("Returning page %d", f.CallCount)
		output := f.IamRolesPageOutputs[f.CallCount]
		f.CallCount++
		return output, nil
	}
	// No more pages.
	return nil, nil
}

func (f *FakeIamClient) ListOpenIDConnectProviders(ctx context.Context, input *iam.ListOpenIDConnectProvidersInput, optFns ...func(*iam.Options)) (*iam.ListOpenIDConnectProvidersOutput, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	if f.CallCount == f.ErrorOnCall {
		return nil, errors.New("list oidc proviers error")
	}
	return f.ListOpenIDConnectProviderPageOutputs, nil
}

// get region
func (f *FakeIamClient) GetRegion() string {
	return f.Region
}
