package core

import (
	"errors"
	"net/url"
	"strings"
)

func Canonicalize(raw string) (canon string, host string, err error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", "", errors.New("url must be absolute with scheme and host")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", "", errors.New("unsupported scheme")
	}

	//lowercase
	h := strings.ToLower(u.Host)

	//strip
	if (scheme == "http" && strings.HasSuffix(h, ":80")) ||
		(scheme == "https" && strings.HasSuffix(h, ":443")) {
		h = strings.Split(h, ":")[0]
	}

	//normalize
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}

	//drop
	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimRight(path, "/")
	}

	u.Scheme = scheme
	u.Host = h
	u.Fragment = ""
	u.Path = path

	return u.String(), h, nil
}
