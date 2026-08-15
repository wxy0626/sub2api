//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/stretchr/testify/require"
)

type credentialAccountRepoStub struct {
	accountRepoStub
	account *Account
	getErr  error
}

func (r *credentialAccountRepoStub) GetByID(_ context.Context, _ int64) (*Account, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.account, nil
}

func TestAdminServiceGetAccountCredential(t *testing.T) {
	t.Run("returns plaintext api_key", func(t *testing.T) {
		repo := &credentialAccountRepoStub{
			account: &Account{
				ID: 1,
				Credentials: map[string]any{
					"api_key": "sk-secret",
					"base_url": "https://api.example.com",
				},
			},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		value, err := svc.GetAccountCredential(context.Background(), 1, "api_key")
		require.NoError(t, err)
		require.Equal(t, "sk-secret", value)
	})

	t.Run("rejects non-sensitive key", func(t *testing.T) {
		repo := &credentialAccountRepoStub{
			account: &Account{
				ID: 1,
				Credentials: map[string]any{
					"base_url": "https://api.example.com",
				},
			},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		_, err := svc.GetAccountCredential(context.Background(), 1, "base_url")
		require.Error(t, err)
		require.Equal(t, http.StatusBadRequest, infraerrors.Code(err))
	})

	t.Run("returns not found when key missing", func(t *testing.T) {
		repo := &credentialAccountRepoStub{
			account: &Account{
				ID:          1,
				Credentials: map[string]any{},
			},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		_, err := svc.GetAccountCredential(context.Background(), 1, "api_key")
		require.Error(t, err)
		require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
	})

	t.Run("returns not found when key empty", func(t *testing.T) {
		repo := &credentialAccountRepoStub{
			account: &Account{
				ID: 1,
				Credentials: map[string]any{
					"api_key": "",
				},
			},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		_, err := svc.GetAccountCredential(context.Background(), 1, "api_key")
		require.Error(t, err)
		require.Equal(t, http.StatusNotFound, infraerrors.Code(err))
	})

	t.Run("returns internal server error when value is not string", func(t *testing.T) {
		repo := &credentialAccountRepoStub{
			account: &Account{
				ID: 1,
				Credentials: map[string]any{
					"api_key": 123,
				},
			},
		}
		svc := &adminServiceImpl{accountRepo: repo}

		_, err := svc.GetAccountCredential(context.Background(), 1, "api_key")
		require.Error(t, err)
		require.Equal(t, http.StatusInternalServerError, infraerrors.Code(err))
	})

	t.Run("returns repository error as-is", func(t *testing.T) {
		repo := &credentialAccountRepoStub{
			getErr: errors.New("db connection failed"),
		}
		svc := &adminServiceImpl{accountRepo: repo}

		_, err := svc.GetAccountCredential(context.Background(), 1, "api_key")
		require.Error(t, err)
		require.ErrorIs(t, err, repo.getErr)
	})
}
