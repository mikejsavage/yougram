package main

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"golang.org/x/text/unicode/norm"
	"time"
)

var cookie_aead cipher.AEAD

func secureRandomBytes( n int ) []byte {
	if n > 32 { panic( "n > 32" ) }
	var bytes [32]byte
	_ = must1( rand.Read( bytes[ :n ] ) )
	return bytes[ :n ]
}

func secureRandomHexString( n int ) string {
	return hex.EncodeToString( secureRandomBytes( n ) )
}

func secureRandomBase64String( n int ) string {
	return base64.URLEncoding.EncodeToString( secureRandomBytes( n ) )
}

func initCookieEncryptionKey() []byte {
	filename := "cookie_encryption_key.bin"
	key, err := os.ReadFile( filename )
	if err == nil && len( key ) == chacha20poly1305.KeySize {
		return key
	}

	if !errors.Is( err, os.ErrNotExist ) {
		must( err )
	}

	key = secureRandomBytes( chacha20poly1305.KeySize )
	must( os.WriteFile( filename, key, 0644 ) )
	return key
}

func initCookieAEAD() {
	cookie_encryption_key := initCookieEncryptionKey()
	cookie_aead = try1( chacha20poly1305.NewX( cookie_encryption_key ) )
}

func encodeAuthCookie( username string, secret []byte ) string {
	if len( secret ) != 16 {
		panic( "bad cookie" )
	}

	nonce := secureRandomBytes( chacha20poly1305.NonceSizeX )
	cookie := append( secret, []byte( username )... )

	return base64.StdEncoding.EncodeToString( append( nonce, cookie_aead.Seal( nil, nonce, cookie, nil )... ) )
}

func decodeAuthCookie( cookie string ) ( username string, secret []byte ) {
	decoded, err := base64.StdEncoding.DecodeString( cookie )
	if err != nil || len( decoded ) < chacha20poly1305.NonceSizeX {
		return
	}

	nonce := decoded[ :chacha20poly1305.NonceSizeX ]
	decrypted, err := cookie_aead.Open( nil, nonce, decoded[ chacha20poly1305.NonceSizeX: ], nil )
	if err != nil {
		return
	}

	if len( decrypted ) < 17 {
		return
	}

	return string( decrypted[ 16: ] ), decrypted[ :16 ]
}

func hashPasswordWithSalt( password string, salt []byte ) string {
	password = norm.NFKC.String( password )

	recommended_time := uint32( 3 )
	_32mb_in_kb := uint32( 32 * 1024 )
	key := argon2.Key( []byte( password ), salt, recommended_time, _32mb_in_kb, 4, 32 )

	return fmt.Sprintf( "%-8s%s%s", "argon2i", hex.EncodeToString( salt ), hex.EncodeToString( key ) )
}

func hashPassword( password string ) string {
	salt := secureRandomBytes( 16 )
	return hashPasswordWithSalt( password, salt )
}

func verifyPassword( password string, hash string ) bool {
	if len( hash ) < 8 {
		return false
	}

	algo := hash[ 0:8 ]
	if algo != "argon2i " {
		return false
	}
	if len( hash ) != 8 + 32 + 64 {
		return false
	}

	salt, err := hex.DecodeString( hash[ 8:40 ] )
	if err != nil {
		return false
	}

	ref_hash := hashPasswordWithSalt( password, salt )
	var ok bool
	subtle.WithDataIndependentTiming( func() {
		ok = subtle.ConstantTimeCompare( []byte( hash ), []byte( ref_hash ) ) == 1
	} )

	return ok
}

func setAuthCookie( w http.ResponseWriter, r *http.Request, value string ) {
	expiration := -1
	if value != "" {
		expiration = int( ( 365 * 24 * time.Hour ).Seconds() )
	}

	cookie := http.Cookie {
		Name: "auth",
		Value: value,
		Path: "/",
		MaxAge: expiration,
		// NOTE: Forwarded: for=192.0.2.60;proto=http;by=203.0.113.43
		Secure: strings.Contains( r.Header.Get( "Forwarded" ), "proto=https" ),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}

	http.SetCookie( w, &cookie )
}

func authenticate( w http.ResponseWriter, r *http.Request ) {
	username := norm.NFKC.String( r.PostFormValue( "username" ) )

	user := queryOptional( queries.GetUserAuthDetails( r.Context(), username ) )
	if !user.Valid || user.V.Enabled == 0 {
		http.Error( w, "Incorrect username", http.StatusOK )
		return
	}

	if !verifyPassword( r.PostFormValue( "password" ), user.V.Password ) {
		http.Error( w, "Incorrect password", http.StatusOK )
		return
	}

	setAuthCookie( w, r, encodeAuthCookie( username, user.V.Cookie ) )
	w.Header().Set( "HX-Refresh", "true" )
}

func logout( w http.ResponseWriter, r *http.Request ) {
	setAuthCookie( w, r, "" )
	http.Redirect( w, r, "/", http.StatusSeeOther )
}

func checkAuth( w http.ResponseWriter, r *http.Request, handler func( http.ResponseWriter, *http.Request, User ) ) bool {
	authed := false
	cookie, err_cookie := r.Cookie( "auth" )

	var user User
	var secret []byte
	if err_cookie == nil {
		var username string
		username, secret = decodeAuthCookie( cookie.Value )
		row := queryOptional( queries.GetUserAuthDetails( r.Context(), username ) )
		if row.Valid && row.V.Enabled == 1 {
			subtle.WithDataIndependentTiming( func() {
				if subtle.ConstantTimeCompare( secret, row.V.Cookie ) == 1 {
					authed = true
					user = User { row.V.ID, username }
				}
			} )
		}
	}

	if !authed {
		return false
	}

	setAuthCookie( w, r, encodeAuthCookie( user.Username, secret ) )
	handler( w, r, user )
	return true
}

func requireAuth( handler func( http.ResponseWriter, *http.Request, User ) ) func( http.ResponseWriter, *http.Request ) {
	return func( w http.ResponseWriter, r *http.Request ) {
		if !checkAuth( w, r, handler ) {
			if r.Method == "GET" {
				w.WriteHeader( http.StatusUnauthorized )
				loginForm( w, r )
			} else {
				httpError( w, http.StatusUnauthorized )
			}
		}
	}
}

func requireAuthNoLoginForm( handler func( http.ResponseWriter, *http.Request, User ) ) func( http.ResponseWriter, *http.Request ) {
	return func( w http.ResponseWriter, r *http.Request ) {
		if !checkAuth( w, r, handler ) {
			httpError( w, http.StatusUnauthorized )
		}
	}
}
