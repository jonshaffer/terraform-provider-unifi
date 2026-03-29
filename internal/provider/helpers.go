package provider

import "github.com/jonshaffer/go-unifi/unifi"

func unifiIsNotFound(err error) bool {
	return unifi.IsNotFound(err)
}
