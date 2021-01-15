package api

import (
	"github.com/labstack/echo"
	"github.com/mcarloai/mai-v3-broker/common/auth"
	"github.com/mcarloai/mai-v3-broker/conf"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func GetJwtAuth(param Param) (interface{}, error) {
	address := param.GetAddress()
	// white list
	if len(conf.Conf.WhiteList) > 0 {
		find := false
		for _, item := range conf.Conf.WhiteList {
			if strings.ToLower(address) == strings.ToLower(item) {
				find = true
				break
			}
		}
		if !find {
			return nil, AddressNotInWhiteListError()
		}
	}

	jwt, err := auth.SignJwt(address)
	if err != nil {
		return "", err
	}
	expires := auth.JwtExpiration / time.Second * 1000
	return map[string]interface{}{
		"jwt":     jwt,
		"expires": expires,
	}, nil
}

func CheckJwtAuthByCookie(c echo.Context) error {
	cookie, err := c.Cookie("mc3a")
	if err != nil {
		return c.String(http.StatusForbidden, "token error")
	}
	token, err := url.QueryUnescape(cookie.String())
	if err != nil {
		return c.String(http.StatusForbidden, "token error")
	}
	address, err := auth.ValidateJwt(token)
	if err != nil {
		return c.String(http.StatusForbidden, "mai auth error")
	}

	// white list
	if len(conf.Conf.WhiteList) > 0 {
		find := false
		for _, item := range conf.Conf.WhiteList {
			if strings.ToLower(address) == strings.ToLower(item) {
				find = true
				break
			}
		}
		if !find {
			return c.String(http.StatusForbidden, "address not in white list")
		}
	}

	return c.String(http.StatusOK, "OK")
}
