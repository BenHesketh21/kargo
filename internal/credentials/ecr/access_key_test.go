package ecr

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"

	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestAccessKeyCredentialHelper(t *testing.T) {
	const (
		testRegion          = "fake-region"
		testAccessKeyID     = "fake-id"
		testSecretAccessKey = "fake-secret"
		testUsername        = "fake-username"
		testPassword        = "fake-password"
	)
	testToken := fmt.Sprintf("%s:%s", testUsername, testPassword)
	testEncodedToken := base64.StdEncoding.EncodeToString([]byte(testToken))

	warmTokenCache := cache.New(0, 0)
	warmTokenCache.Set(
		(&accessKeyCredentialHelper{}).tokenCacheKey(testRegion, testAccessKeyID, testSecretAccessKey),
		testEncodedToken,
		cache.DefaultExpiration,
	)

	testCases := []struct {
		name       string
		secret     *corev1.Secret
		helper     AccessKeyCredentialHelper
		assertions func(t *testing.T, username, password string, c *cache.Cache, err error)
	}{
		{
			name:   "no aws details provided",
			secret: &corev1.Secret{},
			helper: NewAccessKeyCredentialHelper(),
			assertions: func(t *testing.T, username, password string, _ *cache.Cache, err error) {
				require.NoError(t, err)
				require.Empty(t, username)
				require.Empty(t, password)
			},
		},
		{
			name: "region missing",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					idKey:     []byte(testAccessKeyID),
					secretKey: []byte(testSecretAccessKey),
				},
			},
			helper: NewAccessKeyCredentialHelper(),
			assertions: func(t *testing.T, _, _ string, _ *cache.Cache, err error) {
				require.ErrorContains(t, err, "must all be set or all be unset")
			},
		},
		{
			name: "access key id missing",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					regionKey: []byte(testRegion),
					secretKey: []byte(testSecretAccessKey),
				},
			},
			helper: NewAccessKeyCredentialHelper(),
			assertions: func(t *testing.T, _, _ string, _ *cache.Cache, err error) {
				require.ErrorContains(t, err, "must all be set or all be unset")
			},
		},
		{
			name: "secret access key missing",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					regionKey: []byte(testRegion),
					idKey:     []byte(testAccessKeyID),
				},
			},
			helper: NewAccessKeyCredentialHelper(),
			assertions: func(t *testing.T, _, _ string, _ *cache.Cache, err error) {
				require.ErrorContains(t, err, "must all be set or all be unset")
			},
		},
		{
			name: "cache hit",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					regionKey: []byte(testRegion),
					idKey:     []byte(testAccessKeyID),
					secretKey: []byte(testSecretAccessKey),
				},
			},
			helper: &accessKeyCredentialHelper{
				tokenCache: warmTokenCache,
			},
			assertions: func(t *testing.T, username, password string, _ *cache.Cache, err error) {
				require.NoError(t, err)
				require.Equal(t, testUsername, username)
				require.Equal(t, testPassword, password)
			},
		},
		{
			name: "cache miss; error getting auth token",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					regionKey: []byte(testRegion),
					idKey:     []byte(testAccessKeyID),
					secretKey: []byte(testSecretAccessKey),
				},
			},
			helper: &accessKeyCredentialHelper{
				tokenCache: cache.New(0, 0),
				getAuthTokenFn: func(context.Context, string, string, string) (string, error) {
					return "", fmt.Errorf("something went wrong")
				},
			},
			assertions: func(t *testing.T, _, _ string, _ *cache.Cache, err error) {
				require.ErrorContains(t, err, "error getting ECR auth token")
				require.ErrorContains(t, err, "something went wrong")
			},
		},
		{
			name: "cache miss; success",
			secret: &corev1.Secret{
				Data: map[string][]byte{
					regionKey: []byte(testRegion),
					idKey:     []byte(testAccessKeyID),
					secretKey: []byte(testSecretAccessKey),
				},
			},
			helper: &accessKeyCredentialHelper{
				tokenCache: cache.New(0, 0),
				getAuthTokenFn: func(context.Context, string, string, string) (string, error) {
					return testEncodedToken, nil
				},
			},
			assertions: func(t *testing.T, username, password string, c *cache.Cache, err error) {
				require.NoError(t, err)
				require.Equal(t, testUsername, username)
				require.Equal(t, testPassword, password)
				_, found := c.Get(
					(&accessKeyCredentialHelper{}).tokenCacheKey(testRegion, testAccessKeyID, testSecretAccessKey),
				)
				require.True(t, found)
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			username, password, err :=
				testCase.helper.GetUsernameAndPassword(context.Background(), testCase.secret)
			cache := testCase.helper.(*accessKeyCredentialHelper).tokenCache // nolint: forcetypeassert
			testCase.assertions(t, username, password, cache, err)
		})
	}
}
