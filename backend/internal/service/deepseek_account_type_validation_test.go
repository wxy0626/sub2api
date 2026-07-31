//go:build unit

package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// assertDeepSeekTypeError 断言 DeepSeek 非 API Key 类型返回可执行的中文错误说明。
func assertDeepSeekTypeError(t *testing.T, err error) {
	t.Helper()
	require.Error(t, err)
	require.Contains(t, err.Error(), "DeepSeek")
	require.Contains(t, err.Error(), "仅支持 API Key")
	require.Contains(t, err.Error(), "credentials.api_key")
}

// TestAccountServiceDeepSeekTypeValidation 覆盖普通账号服务的创建与更新校验。
func TestAccountServiceDeepSeekTypeValidation(t *testing.T) {
	accountTypes := []string{AccountTypeOAuth, AccountTypeSetupToken}

	for _, accountType := range accountTypes {
		accountType := accountType
		t.Run("Create/"+accountType, func(t *testing.T) {
			// repo 是现有账号服务测试桩，创建失败时不应产生账号记录。
			repo := &upstreamBillingProbeAccountRepo{}
			service := NewAccountService(repo, nil)

			_, err := service.Create(context.Background(), CreateAccountRequest{
				Platform:    PlatformDeepSeek,
				Type:        accountType,
				Credentials: map[string]any{"api_key": "sk-test"},
			})

			assertDeepSeekTypeError(t, err)
			require.Empty(t, repo.accounts)
		})

		t.Run("Update/"+accountType, func(t *testing.T) {
			const accountID int64 = 301
			const originalName = "历史非法账号"
			// 预置历史非法账号，验证更新入口在任何字段写入前拒绝请求。
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
				accountID: {
					ID:       accountID,
					Name:     originalName,
					Platform: PlatformDeepSeek,
					Type:     accountType,
					Status:   StatusActive,
				},
			}}
			service := NewAccountService(repo, nil)

			_, err := service.Update(context.Background(), accountID, UpdateAccountRequest{
				Name:        deepSeekStringPtr("不应写入"),
				Credentials: &map[string]any{"api_key": "sk-test"},
			})

			assertDeepSeekTypeError(t, err)
			require.Equal(t, originalName, repo.accounts[accountID].Name)
		})
	}
}

// TestAdminServiceDeepSeekTypeValidation 覆盖管理员账号服务的创建与更新校验。
func TestAdminServiceDeepSeekTypeValidation(t *testing.T) {
	accountTypes := []string{AccountTypeOAuth, AccountTypeSetupToken}

	for _, accountType := range accountTypes {
		accountType := accountType
		t.Run("CreateAccount/"+accountType, func(t *testing.T) {
			// SkipDefaultGroupBind 避免测试依赖管理员默认分组查询。
			repo := &upstreamBillingProbeAccountRepo{}
			service := &adminServiceImpl{accountRepo: repo}

			_, err := service.CreateAccount(context.Background(), &CreateAccountInput{
				Platform:             PlatformDeepSeek,
				Type:                 accountType,
				Credentials:          map[string]any{"api_key": "sk-test"},
				SkipDefaultGroupBind: true,
			})

			assertDeepSeekTypeError(t, err)
			require.Empty(t, repo.accounts)
		})

		t.Run("UpdateAccount/"+accountType, func(t *testing.T) {
			const accountID int64 = 302
			const originalName = "管理员历史非法账号"
			// 管理员更新从数据库读取账号后，应在更新字段前拦截非法类型。
			repo := &upstreamBillingProbeAccountRepo{accounts: map[int64]*Account{
				accountID: {
					ID:       accountID,
					Name:     originalName,
					Platform: PlatformDeepSeek,
					Type:     accountType,
					Status:   StatusActive,
				},
			}}
			service := &adminServiceImpl{accountRepo: repo}

			_, err := service.UpdateAccount(context.Background(), accountID, &UpdateAccountInput{
				Name:        "不应写入",
				Credentials: map[string]any{"api_key": "sk-test"},
			})

			assertDeepSeekTypeError(t, err)
			require.Equal(t, originalName, repo.accounts[accountID].Name)
		})
	}
}

// TestDeepSeekCredentialValidation 覆盖 DeepSeek 创建和编辑时的 API Key 非空校验。
func TestDeepSeekCredentialValidation(t *testing.T) {
	for _, credentials := range []map[string]any{
		nil,
		{},
		{"api_key": "   "},
		{"api_key": 123},
	} {
		credentials := credentials
		t.Run(fmt.Sprintf("invalid-%T", credentials), func(t *testing.T) {
			err := ValidateAccountPlatformCredentials(PlatformDeepSeek, AccountTypeAPIKey, credentials)
			require.ErrorContains(t, err, "DeepSeek API Key 未配置")
			require.ErrorContains(t, err, "credentials.api_key")
		})
	}
	require.NoError(t, ValidateAccountPlatformCredentials(PlatformDeepSeek, AccountTypeAPIKey, map[string]any{"api_key": "sk-test"}))
}

// deepSeekStringPtr 返回字符串指针，便于构造普通更新请求。
func deepSeekStringPtr(value string) *string {
	return &value
}
