package httputil

import (
	"fmt"
	"io"
	"net/http"
)

// ReadResponseBody reads the full body from an HTTP response, closes it, and
// checks the status code. If the status code is >= 400, it returns an error
// using the provided context string. The response body is always closed before
// returning.
//
// This helper deduplicates the common pattern of defer Body.Close() + io.ReadAll
// + status-code check that appears in proxy, githubhttp, and similar call sites.
func ReadResponseBody(resp *http.Response, context string) ([]byte, error) {
	logHTTP.Printf("ReadResponseBody: context=%s", context)
	if resp == nil {
		logHTTP.Printf("ReadResponseBody: nil response for context=%s", context)
		return nil, fmt.Errorf("failed to read %s response: nil response", context)
	}
	if resp.Body == nil {
		logHTTP.Printf("ReadResponseBody: nil body for context=%s, status=%d", context, resp.StatusCode)
		return nil, fmt.Errorf("failed to read %s response: response body is nil", context)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logHTTP.Printf("ReadResponseBody: body read failed for context=%s, status=%d, err=%v", context, resp.StatusCode, err)
		return nil, fmt.Errorf("failed to read %s response: %w", context, err)
	}

	if resp.StatusCode >= 400 {
		logHTTP.Printf("ReadResponseBody: HTTP error response: context=%s, status=%d, bodyLen=%d", context, resp.StatusCode, len(body))
		return nil, fmt.Errorf("%s returned HTTP %d", context, resp.StatusCode)
	}

	logHTTP.Printf("ReadResponseBody: success: context=%s, status=%d, bodyLen=%d", context, resp.StatusCode, len(body))
	return body, nil
}

// ReadResponseBodyStrict reads the full body from an HTTP response, closes it,
// and checks that the status code equals expectedStatus exactly. If the status
// code does not match, it returns an error that includes the response body for
// diagnostics. The response body is always closed before returning.
//
// Use this variant when the caller needs an exact status match (e.g. 200 only)
// and wants the body included in the error message.
func ReadResponseBodyStrict(resp *http.Response, expectedStatus int, context string) ([]byte, error) {
	logHTTP.Printf("ReadResponseBodyStrict: context=%s, expectedStatus=%d", context, expectedStatus)
	if resp == nil {
		logHTTP.Printf("ReadResponseBodyStrict: nil response for context=%s", context)
		return nil, fmt.Errorf("failed to read %s response: nil response", context)
	}
	if resp.Body == nil {
		logHTTP.Printf("ReadResponseBodyStrict: nil body for context=%s, status=%d", context, resp.StatusCode)
		return nil, fmt.Errorf("failed to read %s response: response body is nil", context)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logHTTP.Printf("ReadResponseBodyStrict: body read failed for context=%s, status=%d, err=%v", context, resp.StatusCode, err)
		return nil, fmt.Errorf("failed to read %s response: %w", context, err)
	}

	if resp.StatusCode != expectedStatus {
		logHTTP.Printf("ReadResponseBodyStrict: unexpected status: context=%s, got=%d, expected=%d, bodyLen=%d", context, resp.StatusCode, expectedStatus, len(body))
		return nil, fmt.Errorf("%s returned HTTP %d: %s", context, resp.StatusCode, string(body))
	}

	logHTTP.Printf("ReadResponseBodyStrict: success: context=%s, status=%d, bodyLen=%d", context, resp.StatusCode, len(body))
	return body, nil
}
