package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/github/github-mcp-server/internal/toolsnaps"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v87/github"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"
)

func Test_ListRepositorySecrets(t *testing.T) {
	toolDef := ListRepositorySecrets(translations.NullTranslationHelper)

	require.NoError(t, toolsnaps.Test(toolDef.Tool.Name, toolDef.Tool))

	assert.Equal(t, "list_repository_secrets", toolDef.Tool.Name)
	assert.NotEmpty(t, toolDef.Tool.Description)
	assert.True(t, toolDef.Tool.Annotations.ReadOnlyHint)

	schema, ok := toolDef.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, "InputSchema should be *jsonschema.Schema")
	assert.Contains(t, schema.Properties, "owner")
	assert.Contains(t, schema.Properties, "repo")
	assert.ElementsMatch(t, schema.Required, []string{"owner", "repo"})

	created := github.Timestamp{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	updated := github.Timestamp{Time: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)}
	mockSecrets := &github.Secrets{
		TotalCount: 1,
		Secrets: []*github.Secret{
			{
				Name:      "EXAMPLE_TOKEN",
				CreatedAt: created,
				UpdatedAt: updated,
			},
		},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]any
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "successful listing",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposActionsSecretsByOwnerByRepo: expectQueryParams(t, map[string]string{
					"page":     "1",
					"per_page": "30",
				}).andThen(
					mockResponse(t, http.StatusOK, mockSecrets),
				),
			}),
			requestArgs: map[string]any{
				"owner": "owner",
				"repo":  "repo",
			},
		},
		{
			name: "listing fails",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposActionsSecretsByOwnerByRepo: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusForbidden)
					_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
				}),
			}),
			requestArgs: map[string]any{
				"owner": "owner",
				"repo":  "repo",
			},
			expectError:    true,
			expectedErrMsg: "failed to list",
		},
		{
			name:         "missing owner",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{}),
			requestArgs: map[string]any{
				"repo": "repo",
			},
			expectError:    true,
			expectedErrMsg: "missing required parameter: owner",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := mustNewGHClient(t, tc.mockedClient)
			deps := BaseDeps{Client: client}
			handler := toolDef.Handler(deps)
			request := createMCPRequest(tc.requestArgs)

			result, err := handler(ContextWithDeps(context.Background(), deps), &request)
			require.NoError(t, err)

			if tc.expectError {
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				return
			}

			require.False(t, result.IsError)
			textContent := getTextResult(t, result)

			var payload struct {
				TotalCount int `json:"total_count"`
				Secrets    []struct {
					Name      string `json:"name"`
					CreatedAt string `json:"created_at"`
					UpdatedAt string `json:"updated_at"`
				} `json:"secrets"`
			}
			require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
			assert.Equal(t, 1, payload.TotalCount)
			require.Len(t, payload.Secrets, 1)
			assert.Equal(t, "EXAMPLE_TOKEN", payload.Secrets[0].Name)
			assert.NotEmpty(t, payload.Secrets[0].CreatedAt)
			assert.NotContains(t, textContent.Text, "value")
		})
	}
}

func Test_GetRepositorySecret(t *testing.T) {
	toolDef := GetRepositorySecret(translations.NullTranslationHelper)

	require.NoError(t, toolsnaps.Test(toolDef.Tool.Name, toolDef.Tool))

	assert.Equal(t, "get_repository_secret", toolDef.Tool.Name)
	assert.True(t, toolDef.Tool.Annotations.ReadOnlyHint)

	schema, ok := toolDef.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, "InputSchema should be *jsonschema.Schema")
	assert.ElementsMatch(t, schema.Required, []string{"owner", "repo", "name"})

	mockSecret := &github.Secret{
		Name:      "EXAMPLE_TOKEN",
		CreatedAt: github.Timestamp{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		UpdatedAt: github.Timestamp{Time: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]any
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "successful get",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposActionsSecretsByOwnerByRepoBySecretName: mockResponse(t, http.StatusOK, mockSecret),
			}),
			requestArgs: map[string]any{
				"owner": "owner",
				"repo":  "repo",
				"name":  "EXAMPLE_TOKEN",
			},
		},
		{
			name: "secret not found",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposActionsSecretsByOwnerByRepoBySecretName: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				}),
			}),
			requestArgs: map[string]any{
				"owner": "owner",
				"repo":  "repo",
				"name":  "MISSING",
			},
			expectError:    true,
			expectedErrMsg: "failed to get",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := mustNewGHClient(t, tc.mockedClient)
			deps := BaseDeps{Client: client}
			handler := toolDef.Handler(deps)
			request := createMCPRequest(tc.requestArgs)

			result, err := handler(ContextWithDeps(context.Background(), deps), &request)
			require.NoError(t, err)

			if tc.expectError {
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				return
			}

			require.False(t, result.IsError)
			textContent := getTextResult(t, result)

			var payload repositorySecretMetadata
			require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
			assert.Equal(t, "EXAMPLE_TOKEN", payload.Name)
			assert.NotContains(t, textContent.Text, "super-secret")
		})
	}
}

func Test_SetRepositorySecret(t *testing.T) {
	toolDef := SetRepositorySecret(translations.NullTranslationHelper)

	require.NoError(t, toolsnaps.Test(toolDef.Tool.Name, toolDef.Tool))

	assert.Equal(t, "set_repository_secret", toolDef.Tool.Name)
	assert.False(t, toolDef.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, toolDef.Tool.Annotations.DestructiveHint)
	assert.False(t, *toolDef.Tool.Annotations.DestructiveHint)

	schema, ok := toolDef.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, "InputSchema should be *jsonschema.Schema")
	assert.ElementsMatch(t, schema.Required, []string{"owner", "repo", "name", "value"})

	publicKey, _, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)
	publicKeyB64 := base64.StdEncoding.EncodeToString(publicKey[:])

	tests := []struct {
		name           string
		status         int
		expectError    bool
		expectedOp     string
		expectedErrMsg string
	}{
		{name: "creates secret", status: http.StatusCreated, expectedOp: "created"},
		{name: "updates secret", status: http.StatusNoContent, expectedOp: "updated"},
		{name: "set fails", status: http.StatusForbidden, expectError: true, expectedErrMsg: "failed to set"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedBody map[string]any
			mockedClient := MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposActionsSecretsPublicKeyByOwnerByRepo: mockResponse(t, http.StatusOK, &github.PublicKey{
					KeyID: github.Ptr("test-key-id"),
					Key:   github.Ptr(publicKeyB64),
				}),
				PutReposActionsSecretsByOwnerByRepoBySecretName: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					require.NoError(t, json.NewDecoder(r.Body).Decode(&capturedBody))
					w.WriteHeader(tc.status)
					if tc.status >= 400 {
						_, _ = w.Write([]byte(`{"message": "Forbidden"}`))
					}
				}),
			})

			client := mustNewGHClient(t, mockedClient)
			deps := BaseDeps{Client: client}
			handler := toolDef.Handler(deps)
			request := createMCPRequest(map[string]any{
				"owner": "owner",
				"repo":  "repo",
				"name":  "EXAMPLE_TOKEN",
				"value": "test-secret-value",
			})

			result, err := handler(ContextWithDeps(context.Background(), deps), &request)
			require.NoError(t, err)

			if tc.expectError {
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				assert.NotContains(t, errorContent.Text, "test-secret-value")
				return
			}

			require.False(t, result.IsError)
			textContent := getTextResult(t, result)
			assert.NotContains(t, textContent.Text, "test-secret-value")

			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
			assert.Equal(t, "EXAMPLE_TOKEN", payload["name"])
			assert.Equal(t, tc.expectedOp, payload["operation"])

			require.NotNil(t, capturedBody)
			assert.Equal(t, "test-key-id", capturedBody["key_id"])
			encrypted, ok := capturedBody["encrypted_value"].(string)
			require.True(t, ok)
			assert.NotEmpty(t, encrypted)
			assert.NotEqual(t, "test-secret-value", encrypted)
			assert.NotContains(t, encrypted, "test-secret-value")
		})
	}
}

func Test_DeleteRepositorySecret(t *testing.T) {
	toolDef := DeleteRepositorySecret(translations.NullTranslationHelper)

	require.NoError(t, toolsnaps.Test(toolDef.Tool.Name, toolDef.Tool))

	assert.Equal(t, "delete_repository_secret", toolDef.Tool.Name)
	assert.False(t, toolDef.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, toolDef.Tool.Annotations.DestructiveHint)
	assert.True(t, *toolDef.Tool.Annotations.DestructiveHint)

	schema, ok := toolDef.Tool.InputSchema.(*jsonschema.Schema)
	require.True(t, ok, "InputSchema should be *jsonschema.Schema")
	assert.ElementsMatch(t, schema.Required, []string{"owner", "repo", "name"})

	tests := []struct {
		name           string
		mockedClient   *http.Client
		expectError    bool
		expectedErrMsg string
	}{
		{
			name: "successful delete",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				DeleteReposActionsSecretsByOwnerByRepoBySecretName: mockResponse(t, http.StatusNoContent, ""),
			}),
		},
		{
			name: "delete fails",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				DeleteReposActionsSecretsByOwnerByRepoBySecretName: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				}),
			}),
			expectError:    true,
			expectedErrMsg: "failed to delete",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := mustNewGHClient(t, tc.mockedClient)
			deps := BaseDeps{Client: client}
			handler := toolDef.Handler(deps)
			request := createMCPRequest(map[string]any{
				"owner": "owner",
				"repo":  "repo",
				"name":  "EXAMPLE_TOKEN",
			})

			result, err := handler(ContextWithDeps(context.Background(), deps), &request)
			require.NoError(t, err)

			if tc.expectError {
				require.True(t, result.IsError)
				errorContent := getErrorResult(t, result)
				assert.Contains(t, errorContent.Text, tc.expectedErrMsg)
				return
			}

			require.False(t, result.IsError)
			textContent := getTextResult(t, result)
			var payload map[string]any
			require.NoError(t, json.Unmarshal([]byte(textContent.Text), &payload))
			assert.Equal(t, "EXAMPLE_TOKEN", payload["name"])
			assert.Equal(t, true, payload["deleted"])
		})
	}
}

func Test_encryptSecretWithPublicKey(t *testing.T) {
	publicKey, privateKey, err := box.GenerateKey(rand.Reader)
	require.NoError(t, err)

	plaintext := "test-secret-value"
	encrypted, err := encryptSecretWithPublicKey(base64.StdEncoding.EncodeToString(publicKey[:]), plaintext)
	require.NoError(t, err)
	assert.NotEqual(t, plaintext, encrypted)

	decoded, err := base64.StdEncoding.DecodeString(encrypted)
	require.NoError(t, err)

	opened, ok := box.OpenAnonymous(nil, decoded, publicKey, privateKey)
	require.True(t, ok)
	assert.Equal(t, plaintext, string(opened))

	_, err = encryptSecretWithPublicKey("not-valid-base64!!!", plaintext)
	require.Error(t, err)

	_, err = encryptSecretWithPublicKey(base64.StdEncoding.EncodeToString([]byte("short")), plaintext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid public key length")
}
