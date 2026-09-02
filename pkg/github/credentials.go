package github

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	ghErrors "github.com/github/github-mcp-server/pkg/errors"
	"github.com/github/github-mcp-server/pkg/ifc"
	"github.com/github/github-mcp-server/pkg/inventory"
	"github.com/github/github-mcp-server/pkg/scopes"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/github/github-mcp-server/pkg/utils"
	"github.com/google/go-github/v87/github"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/crypto/nacl/box"
)

// repositorySecretMetadata is the tool response for a repository Actions secret.
// GitHub never returns the secret value — only the name and timestamps.
type repositorySecretMetadata struct {
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ListRepositorySecrets lists GitHub Actions secrets stored on a repository.
// Only secret names and timestamps are returned; values are never available.
func ListRepositorySecrets(t translations.TranslationHelperFunc) inventory.ServerTool {
	schema := &jsonschema.Schema{
		Type: "object",
		Properties: map[string]*jsonschema.Schema{
			"owner": {
				Type:        "string",
				Description: "Repository owner",
			},
			"repo": {
				Type:        "string",
				Description: "Repository name",
			},
		},
		Required: []string{"owner", "repo"},
	}
	WithPagination(schema)

	return NewTool(
		ToolsetMetadataCredentials,
		mcp.Tool{
			Name:        "list_repository_secrets",
			Description: t("TOOL_LIST_REPOSITORY_SECRETS_DESCRIPTION", "List GitHub Actions secrets stored in a repository. Returns secret names and timestamps only — secret values are never available through the API."),
			Annotations: &mcp.ToolAnnotations{
				Title:        t("TOOL_LIST_REPOSITORY_SECRETS_USER_TITLE", "List repository secrets"),
				ReadOnlyHint: true,
			},
			InputSchema: schema,
		},
		[]scopes.Scope{scopes.Repo},
		func(ctx context.Context, deps ToolDependencies, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			owner, err := RequiredParam[string](args, "owner")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			repo, err := RequiredParam[string](args, "repo")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			pagination, err := OptionalPaginationParams(args)
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}

			client, err := deps.GetClient(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			secrets, resp, err := client.Actions.ListRepoSecrets(ctx, owner, repo, &github.ListOptions{
				Page:    pagination.Page,
				PerPage: pagination.PerPage,
			})
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to list secrets for repository '%s/%s'", owner, repo),
					resp,
					err,
				), nil, nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return ghErrors.NewGitHubAPIStatusErrorResponse(ctx, "failed to list repository secrets", resp, body), nil, nil
			}

			payload := map[string]any{
				"total_count": secrets.TotalCount,
				"secrets":     toRepositorySecretMetadataList(secrets.Secrets),
			}
			r, err := json.Marshal(payload)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal secrets: %w", err)
			}

			result := utils.NewToolResultText(string(r))
			result = attachStaticIFCLabel(ctx, deps, result, ifc.LabelRepositorySecrets())
			return result, nil, nil
		},
	)
}

// GetRepositorySecret returns metadata for a single repository Actions secret.
// The secret value is never returned.
func GetRepositorySecret(t translations.TranslationHelperFunc) inventory.ServerTool {
	return NewTool(
		ToolsetMetadataCredentials,
		mcp.Tool{
			Name:        "get_repository_secret",
			Description: t("TOOL_GET_REPOSITORY_SECRET_DESCRIPTION", "Get metadata for a GitHub Actions secret stored in a repository. Returns the secret name and timestamps only — the secret value is never available through the API."),
			Annotations: &mcp.ToolAnnotations{
				Title:        t("TOOL_GET_REPOSITORY_SECRET_USER_TITLE", "Get repository secret"),
				ReadOnlyHint: true,
			},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"owner": {
						Type:        "string",
						Description: "Repository owner",
					},
					"repo": {
						Type:        "string",
						Description: "Repository name",
					},
					"name": {
						Type:        "string",
						Description: "Secret name",
					},
				},
				Required: []string{"owner", "repo", "name"},
			},
		},
		[]scopes.Scope{scopes.Repo},
		func(ctx context.Context, deps ToolDependencies, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			owner, err := RequiredParam[string](args, "owner")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			repo, err := RequiredParam[string](args, "repo")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			name, err := RequiredParam[string](args, "name")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}

			client, err := deps.GetClient(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			secret, resp, err := client.Actions.GetRepoSecret(ctx, owner, repo, name)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to get secret %q in repository '%s/%s'", name, owner, repo),
					resp,
					err,
				), nil, nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return ghErrors.NewGitHubAPIStatusErrorResponse(ctx, "failed to get repository secret", resp, body), nil, nil
			}

			r, err := json.Marshal(toRepositorySecretMetadata(secret))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal secret: %w", err)
			}

			result := utils.NewToolResultText(string(r))
			result = attachStaticIFCLabel(ctx, deps, result, ifc.LabelRepositorySecrets())
			return result, nil, nil
		},
	)
}

// SetRepositorySecret creates or updates a GitHub Actions secret on a repository.
// The plaintext value is encrypted with the repository public key before it is
// sent to GitHub and is never included in the tool response.
func SetRepositorySecret(t translations.TranslationHelperFunc) inventory.ServerTool {
	return NewTool(
		ToolsetMetadataCredentials,
		mcp.Tool{
			Name:        "set_repository_secret",
			Description: t("TOOL_SET_REPOSITORY_SECRET_DESCRIPTION", "Create or update a GitHub Actions secret stored in a repository. The value is encrypted with the repository public key before it is sent to GitHub. The plaintext value is never returned."),
			Annotations: &mcp.ToolAnnotations{
				Title:           t("TOOL_SET_REPOSITORY_SECRET_USER_TITLE", "Set repository secret"),
				ReadOnlyHint:    false,
				DestructiveHint: github.Ptr(false),
			},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"owner": {
						Type:        "string",
						Description: "Repository owner",
					},
					"repo": {
						Type:        "string",
						Description: "Repository name",
					},
					"name": {
						Type:        "string",
						Description: "Secret name",
					},
					"value": {
						Type:        "string",
						Description: "Secret value to store. Encrypted locally with the repository public key before it is sent to GitHub. Never returned by subsequent reads.",
					},
				},
				Required: []string{"owner", "repo", "name", "value"},
			},
		},
		[]scopes.Scope{scopes.Repo},
		func(ctx context.Context, deps ToolDependencies, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			owner, err := RequiredParam[string](args, "owner")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			repo, err := RequiredParam[string](args, "repo")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			name, err := RequiredParam[string](args, "name")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			value, err := RequiredParam[string](args, "value")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}

			client, err := deps.GetClient(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			pubKey, resp, err := client.Actions.GetRepoPublicKey(ctx, owner, repo)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to get public key for repository '%s/%s'", owner, repo),
					resp,
					err,
				), nil, nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusOK {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return ghErrors.NewGitHubAPIStatusErrorResponse(ctx, "failed to get repository public key", resp, body), nil, nil
			}

			encryptedValue, err := encryptSecretWithPublicKey(pubKey.GetKey(), value)
			if err != nil {
				return utils.NewToolResultError("failed to encrypt secret value"), nil, nil
			}

			resp, err = client.Actions.CreateOrUpdateRepoSecret(ctx, owner, repo, &github.EncryptedSecret{
				Name:           name,
				KeyID:          pubKey.GetKeyID(),
				EncryptedValue: encryptedValue,
			})
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to set secret %q in repository '%s/%s'", name, owner, repo),
					resp,
					err,
				), nil, nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return ghErrors.NewGitHubAPIStatusErrorResponse(ctx, "failed to set repository secret", resp, body), nil, nil
			}

			operation := "updated"
			if resp.StatusCode == http.StatusCreated {
				operation = "created"
			}

			r, err := json.Marshal(map[string]any{
				"name":      name,
				"operation": operation,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			result := utils.NewToolResultText(string(r))
			result = attachStaticIFCLabel(ctx, deps, result, ifc.LabelRepositorySecrets())
			return result, nil, nil
		},
	)
}

// DeleteRepositorySecret deletes a GitHub Actions secret from a repository.
func DeleteRepositorySecret(t translations.TranslationHelperFunc) inventory.ServerTool {
	return NewTool(
		ToolsetMetadataCredentials,
		mcp.Tool{
			Name:        "delete_repository_secret",
			Description: t("TOOL_DELETE_REPOSITORY_SECRET_DESCRIPTION", "Delete a GitHub Actions secret stored in a repository."),
			Annotations: &mcp.ToolAnnotations{
				Title:           t("TOOL_DELETE_REPOSITORY_SECRET_USER_TITLE", "Delete repository secret"),
				ReadOnlyHint:    false,
				DestructiveHint: github.Ptr(true),
			},
			InputSchema: &jsonschema.Schema{
				Type: "object",
				Properties: map[string]*jsonschema.Schema{
					"owner": {
						Type:        "string",
						Description: "Repository owner",
					},
					"repo": {
						Type:        "string",
						Description: "Repository name",
					},
					"name": {
						Type:        "string",
						Description: "Secret name",
					},
				},
				Required: []string{"owner", "repo", "name"},
			},
		},
		[]scopes.Scope{scopes.Repo},
		func(ctx context.Context, deps ToolDependencies, _ *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			owner, err := RequiredParam[string](args, "owner")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			repo, err := RequiredParam[string](args, "repo")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}
			name, err := RequiredParam[string](args, "name")
			if err != nil {
				return utils.NewToolResultError(err.Error()), nil, nil
			}

			client, err := deps.GetClient(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			resp, err := client.Actions.DeleteRepoSecret(ctx, owner, repo, name)
			if err != nil {
				return ghErrors.NewGitHubAPIErrorResponse(ctx,
					fmt.Sprintf("failed to delete secret %q in repository '%s/%s'", name, owner, repo),
					resp,
					err,
				), nil, nil
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != http.StatusNoContent {
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to read response body: %w", err)
				}
				return ghErrors.NewGitHubAPIStatusErrorResponse(ctx, "failed to delete repository secret", resp, body), nil, nil
			}

			r, err := json.Marshal(map[string]any{
				"name":    name,
				"deleted": true,
			})
			if err != nil {
				return nil, nil, fmt.Errorf("failed to marshal response: %w", err)
			}

			result := utils.NewToolResultText(string(r))
			result = attachStaticIFCLabel(ctx, deps, result, ifc.LabelRepositorySecrets())
			return result, nil, nil
		},
	)
}

func toRepositorySecretMetadata(secret *github.Secret) repositorySecretMetadata {
	if secret == nil {
		return repositorySecretMetadata{}
	}
	meta := repositorySecretMetadata{Name: secret.Name}
	if !secret.CreatedAt.Time.IsZero() {
		meta.CreatedAt = secret.CreatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if !secret.UpdatedAt.Time.IsZero() {
		meta.UpdatedAt = secret.UpdatedAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return meta
}

func toRepositorySecretMetadataList(secrets []*github.Secret) []repositorySecretMetadata {
	out := make([]repositorySecretMetadata, 0, len(secrets))
	for _, secret := range secrets {
		out = append(out, toRepositorySecretMetadata(secret))
	}
	return out
}

// encryptSecretWithPublicKey encrypts a secret value with a GitHub Actions
// repository public key using libsodium sealed box (nacl box SealAnonymous).
// The returned string is the base64-encoded ciphertext expected by the API.
func encryptSecretWithPublicKey(publicKeyB64, secretValue string) (string, error) {
	decodedKey, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil {
		return "", fmt.Errorf("failed to decode public key: %w", err)
	}
	if len(decodedKey) != 32 {
		return "", fmt.Errorf("invalid public key length: got %d, want 32", len(decodedKey))
	}

	var pk [32]byte
	copy(pk[:], decodedKey)

	sealed, err := box.SealAnonymous(nil, []byte(secretValue), &pk, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt secret: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}
