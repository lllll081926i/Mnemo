package pikpak

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"mnemo-go/internal/drive"
	"mnemo-go/internal/model"
	"mnemo-go/internal/netx"
)

func runFavoriteOperation(ctx context.Context, c drive.Context, operation string) error {
	d := &Driver{}
	if operation == "list" {
		_, err := d.ListFavorites(ctx, c)
		return err
	}
	_, err := d.Favorite(ctx, c, []string{"file"}, operation == "star")
	return err
}

func TestFavoritesRefreshExpiredSessionOnce(t *testing.T) {
	for _, operation := range []string{"list", "star", "unstar"} {
		for _, status := range []int{http.StatusUnauthorized, http.StatusOK} {
			t.Run(operation+http.StatusText(status), func(t *testing.T) {
				previous := netx.TestTransportHook
				t.Cleanup(func() { netx.TestTransportHook = previous; drive.SetTokenUpdater(nil) })
				token := &model.TokenInfo{AccessToken: "old", RefreshToken: "refresh", DeviceID: "device"}
				refreshes, operations, saves := 0, 0, 0
				drive.SetTokenUpdater(func(_, _ string, saved *model.TokenInfo) error {
					saves++
					if saved.AccessToken != "new" || saved.RefreshToken != "rotated" {
						t.Fatal("未保存刷新后的凭据")
					}
					return nil
				})
				netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/v1/auth/token":
						refreshes++
						return pikpakResponse(req, http.StatusOK, `{"access_token":"new","refresh_token":"rotated"}`), nil
					case "/drive/v1/about":
						return pikpakResponse(req, http.StatusOK, `{"quota":{"limit":"100","usage":"10"}}`), nil
					case "/drive/v1/files", "/drive/v1/files:star", "/drive/v1/files:unstar":
						operations++
						if req.Header.Get("Authorization") == "Bearer old" {
							return pikpakResponse(req, status, `{"error":"unauthenticated","error_code":16,"error_description":"Unauthenticated, please re-login"}`), nil
						}
						if req.Header.Get("Authorization") != "Bearer new" {
							t.Fatal("重试未使用新凭据")
						}
						if operation == "list" {
							return pikpakResponse(req, http.StatusOK, `{"files":[{"id":"file","name":"movie.mkv","system_tag":"STAR"}]}`), nil
						}
						return pikpakResponse(req, http.StatusOK, `{}`), nil
					default:
						t.Fatalf("意外请求：%s", req.URL)
						return nil, errors.New("意外请求")
					}
				})
				if err := runFavoriteOperation(t.Context(), drive.Context{UserID: "pikpak_session", DriveID: "space", Token: token}, operation); err != nil {
					t.Fatal(err)
				}
				if refreshes != 1 || operations != 2 || saves != 1 {
					t.Fatalf("刷新=%d，收藏请求=%d，凭据保存=%d", refreshes, operations, saves)
				}
			})
		}
	}
}

func TestFavoritesDoNotLoopOrRefreshOtherFailures(t *testing.T) {
	for _, operation := range []string{"list", "star", "unstar"} {
		for _, tc := range []struct {
			name   string
			status int
			body   string
			auth   bool
		}{
			{"持续登录失效", 401, `{"error":"unauthenticated"}`, true},
			{"权限不足", 403, `{"error":"permission_denied"}`, false},
			{"限流", 429, `{"error":"too_many_requests"}`, false},
			{"服务异常", 503, `{"error":"unavailable"}`, false},
			{"业务拒绝", 200, `{"error":"permission_denied","error_code":7}`, false},
			{"响应损坏", 200, `{"files":`, false},
		} {
			t.Run(operation+tc.name, func(t *testing.T) {
				previous := netx.TestTransportHook
				t.Cleanup(func() { netx.TestTransportHook = previous })
				refreshes, operations := 0, 0
				netx.TestTransportHook = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
					switch req.URL.Path {
					case "/v1/auth/token":
						refreshes++
						return pikpakResponse(req, http.StatusOK, `{"access_token":"new"}`), nil
					case "/drive/v1/about":
						return pikpakResponse(req, http.StatusOK, `{"quota":{}}`), nil
					default:
						operations++
						return pikpakResponse(req, tc.status, tc.body), nil
					}
				})
				token := &model.TokenInfo{AccessToken: "old", RefreshToken: "refresh", DeviceID: "device"}
				err := runFavoriteOperation(t.Context(), drive.Context{UserID: "pikpak_failures", DriveID: "space", Token: token}, operation)
				if err == nil || errors.Is(err, drive.ErrUnauthorized) != tc.auth {
					t.Fatalf("错误分类不正确：%v", err)
				}
				wantRefreshes := 0
				if tc.auth {
					wantRefreshes = 1
				}
				if refreshes != wantRefreshes || operations != 1+wantRefreshes {
					t.Fatalf("刷新=%d，收藏请求=%d", refreshes, operations)
				}
			})
		}
	}
}

func TestFavoriteTruncatedResponseIsNotSuccess(t *testing.T) {
	c := newClient("access", "device", "account")
	c.http.HTTP.Transport = pikpakRoundTripper(func(req *http.Request) (*http.Response, error) {
		response := pikpakResponse(req, http.StatusOK, `{}`)
		response.Body = io.NopCloser(io.MultiReader(strings.NewReader(`{}`), failingFavoriteReader{}))
		return response, nil
	})
	if err := c.Star(t.Context(), []string{"file"}, true); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("回执读取失败未传播：%v", err)
	}
}

type failingFavoriteReader struct{}

func (failingFavoriteReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
