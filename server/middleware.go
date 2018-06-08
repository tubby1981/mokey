// Copyright 2015 mokey Authors. All rights reserved.
// Use of this source code is governed by a BSD style
// license that can be found in the LICENSE file.

package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/gomodule/redigo/redis"
	"github.com/labstack/echo"
	"github.com/labstack/echo-contrib/session"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/ubccr/goipa"
)

// AuthRequired ensures the user has successfully completed the authentication
// process including any 2FA.
func AuthRequired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get(CookieKeySession, c)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get session")
		}

		auth := sess.Values[CookieKeyAuthenticated]

		if auth != nil && auth.(bool) {
			return next(c)
		}

		return c.Redirect(http.StatusFound, "/auth/login")
	}
}

// LoginRequired ensure the user has logged in and has a valid FreeIPA session.
// Stores the ipa.UserRecord in the request context
func LoginRequired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// Check if consent endpoint and Api Key was given
		accept := c.Request().Header.Get("Accept")
		auth := strings.Split(c.Request().Header.Get("Authorization"), " ")
		if strings.HasPrefix(c.Request().URL.String(), "/consent") &&
			strings.Contains(accept, "application/json") &&
			len(auth) == 2 && strings.ToLower(auth[0]) == "bearer" &&
			len(auth[1]) > 0 {
			return next(c)
		}

		sess, err := session.Get(CookieKeySession, c)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get session")
		}

		if !strings.HasPrefix(c.Request().URL.String(), "/auth") {
			query := c.Request().URL.Query().Encode()
			if len(query) > 0 {
				sess.Values[CookieKeyWYAF] = c.Request().URL.Path + "?" + query
			} else {
				sess.Values[CookieKeyWYAF] = c.Request().URL.Path
			}
			log.WithFields(log.Fields{
				"wyaf": sess.Values[CookieKeyWYAF],
			}).Info("Redirect URL")
			sess.Save(c.Request(), c.Response())
		}

		sid := sess.Values[CookieKeySID]
		user := sess.Values[CookieKeyUser]

		if sid == nil || user == nil {
			return c.Redirect(http.StatusFound, "/auth/login")
		}

		if _, ok := user.(string); !ok {
			logout(c)
			log.Error("Invalid user record in session.")
			return c.Redirect(http.StatusFound, "/auth/login")
		}

		client := ipa.NewDefaultClientWithSession(sid.(string))

		userRec, err := client.UserShow(user.(string))
		if err != nil {
			log.WithFields(log.Fields{
				"uid":              user,
				"ipa_client_error": err,
			}).Error("Failed to fetch user info from FreeIPA")
			return c.Redirect(http.StatusFound, "/auth/login")
		}

		c.Set(ContextKeyUser, userRec)
		c.Set("ipa", c)

		return next(c)
	}
}

// UserNameRequired ensure the user has submitted a valid username.
func UserNameRequired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sess, err := session.Get(CookieKeySession, c)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, "Failed to get session")
		}

		user := sess.Values[CookieKeyUser]

		if user == nil {
			logout(c)
			return c.Redirect(http.StatusFound, "/auth/login")
		}

		if _, ok := user.(string); !ok {
			logout(c)
			log.Error("Invalid user record in session.")
			return c.Redirect(http.StatusFound, "/auth/login")
		}

		return next(c)
	}
}

func RateLimit(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		// check if rate limiting is enabled
		if !viper.GetBool("rate_limit") {
			return next(c)
		}

		// only rate limit POST request
		if c.Request().Method != "POST" {
			return next(c)
		}

		remoteIP := c.Request().Header.Get("X-Forwarded-For")
		if len(remoteIP) == 0 {
			remoteIP, _, _ = net.SplitHostPort(c.Request().RemoteAddr)
		}
		path := c.Request().URL.Path

		conn, err := redis.Dial("tcp", viper.GetString("redis"))
		if err != nil {
			log.WithFields(log.Fields{
				"path":     path,
				"remoteIP": remoteIP,
				"err":      err.Error(),
			}).Error("Failed connecting to redis server")
			return next(c)
		}
		defer conn.Close()

		current, err := redis.Int(conn.Do("INCR", path+remoteIP))
		if err != nil {
			log.WithFields(log.Fields{
				"path":     path,
				"remoteIP": remoteIP,
				"err":      err.Error(),
			}).Error("Failed to increment counter in redis")
			return next(c)
		}

		if current > viper.GetInt("max_requests") {
			log.WithFields(log.Fields{
				"path":     path,
				"remoteIP": remoteIP,
				"counter":  current,
			}).Warn("Too many connections")
			return echo.NewHTTPError(http.StatusTooManyRequests, "Too many connections")
		}

		if current == 1 {
			_, err := conn.Do("SETEX", path+remoteIP, viper.GetInt("rate_limit_expire"), 1)
			if err != nil {
				log.WithFields(log.Fields{
					"path":     path,
					"remoteIP": remoteIP,
					"err":      err.Error(),
				}).Error("Failed to set expiry on counter in redis")
			}
		}

		log.WithFields(log.Fields{
			"path":     path,
			"remoteIP": remoteIP,
			"counter":  current,
		}).Info("rate limiting")

		return next(c)
	}
}
