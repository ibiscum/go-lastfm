package lastfm

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

func requireAuth(creds *credentials) (err error) {
	if creds.sk == "" {
		err = newLibError(
			ErrorAuthRequired,
			Messages[ErrorAuthRequired],
		)
	}
	return
}

/*
func checkRequiredParams(params P, required ...string) (err error) {
    var missing []string
    ng := false
    for _, p := range required {
        if _, ok := params[p]; !ok {
            missing = append(missing, p)
            ng = true
        }
    }
    if ng {
        err = newLibError(
            ErrorParameterMissing,
            fmt.Sprintf(Messages[ErrorParameterMissing], required, missing),
        )
    }
    return
}
*/

func constructUrl(base string, params url.Values) (u string) {
	if ResponseFormat == "json" {
		params.Add("format", ResponseFormat)
	}
	p := params.Encode()
	u = base + "?" + p
	return
}

func toString(val interface{}) (str string, err error) {
	switch val.(type) {
	case string:
		str = val.(string)
	case int:
		str = strconv.Itoa(val.(int))
	case []string:
		ss := val.([]string)
		if len(ss) > 10 {
			ss = ss[:10]
		}
		str = strings.Join(ss, ",")
	default:
		err = newLibError(
			ErrorInvalidTypeOfArgument,
			Messages[ErrorInvalidTypeOfArgument],
		)
	}
	return
}

func parseResponse(body []byte, result interface{}) (err error) {
	var base Base
	err = xml.Unmarshal(body, &base)
	if err != nil {
		return
	}
	if base.Status == ApiResponseStatusFailed {
		var errorDetail ApiError
		err = xml.Unmarshal(base.Inner, &errorDetail)
		if err != nil {
			return
		}
		err = newApiError(&errorDetail)
		return
	} else if result == nil {
		return
	}
	err = xml.Unmarshal(base.Inner, result)
	return
}

func getSignature(params map[string]string, secret string) (sig string) {
	var keys []string
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sigPlain string
	for _, k := range keys {
		sigPlain += k + params[k]
	}
	sigPlain += secret

	hasher := md5.New()
	hasher.Write([]byte(sigPlain))
	sig = hex.EncodeToString(hasher.Sum(nil))
	return
}

func formatArgs(args, rules P) (result map[string]string, err error) {

	result = make(map[string]string)
	if _, ok := rules["indexing"]; ok {

		for _, p := range rules["indexing"].([]string) {
			if valI, ok := args[p]; ok {
				switch valI.(type) {
				case string:
					key := p + "[0]"
					val := valI.(string)
					result[key] = val
				case int:
					key := p + "[0]"
					val := strconv.Itoa(valI.(int))
					result[key] = val
				case []string: //with indeces
					for i, val := range valI.([]string) {
						key := fmt.Sprintf("%s[%d]", p, i)
						result[key] = val
					}
				default:
					err = newLibError(
						ErrorInvalidTypeOfArgument,
						Messages[ErrorInvalidTypeOfArgument],
					)
					break
				}
			} else if _, ok := args[p+"[0]"]; ok {
				for i := 0; ; i++ {
					key := fmt.Sprintf("%s[%d]", p, i)
					if valI, ok := args[key]; ok {
						var val string
						switch valI.(type) {
						case string:
							val = valI.(string)
						case int:
							val = strconv.Itoa(valI.(int))
						default:
							err = newLibError(
								ErrorInvalidTypeOfArgument,
								Messages[ErrorInvalidTypeOfArgument],
							)
							break
						}
						result[key] = val
					}
				}
			}
			if err != nil {
				break
			}
		}
	}
	if err != nil {
		return
	}

	if _, ok := rules["normal"]; ok {
		for _, key := range rules["normal"].([]string) {
			if valI, ok := args[key]; ok {
				var val string
				switch valI.(type) {
				case string:
					val = valI.(string)
				case int:
					val = strconv.Itoa(valI.(int))
				case []string: //comma delimited
					ss := valI.([]string)
					if len(ss) > 10 {
						ss = ss[:10]
					}
					val = strings.Join(ss, ",")
				default:
					err = newLibError(
						ErrorInvalidTypeOfArgument,
						Messages[ErrorInvalidTypeOfArgument],
					)
					break
				}
				result[key] = val
			}
		}
	}
	if err != nil {
		return
	}
	return
}

/////////////
// GET API //
/////////////
func callGet(apiMethod string, creds *credentials, args P, result interface{}, rules P) (err error) {
	urlParams := url.Values{}
	urlParams.Add("method", apiMethod)
	urlParams.Add("api_key", creds.apikey)

	formated, err := formatArgs(args, rules)
	if err != nil {
		return
	}
	for k, v := range formated {
		urlParams.Add(k, v)
	}

	uri := constructUrl(UriApiBase, urlParams)

	res, err := http.Get(uri)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	err = parseResponse(body, result)
	return
}

////////////
//POST API//
////////////

func callPost(apiMethod string, creds *credentials, args P, result interface{}, rules P) (err error) {
	if err = requireAuth(creds); err != nil {
		return
	}

	urlParams := url.Values{}
	urlParams.Add("method", apiMethod)
	uri := constructUrl(UriApiSecBase, urlParams)

	//post data
	postData := url.Values{}
	postData.Add("method", apiMethod)
	postData.Add("api_key", creds.apikey)
	postData.Add("sk", creds.sk)

	tmp := make(map[string]string)
	tmp["method"] = apiMethod
	tmp["api_key"] = creds.apikey
	tmp["sk"] = creds.sk

	formated, err := formatArgs(args, rules)
	for k, v := range formated {
		tmp[k] = v
		postData.Add(k, v)
	}

	sig := getSignature(tmp, creds.secret)
	postData.Add("api_sig", sig)

	res, err := http.PostForm(uri, postData)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	err = parseResponse(body, result)
	return
}

func callPostWithoutSession(apiMethod string, creds *credentials, args P, result interface{}, rules P) (err error) {
	urlParams := url.Values{}
	urlParams.Add("method", apiMethod)
	uri := constructUrl(UriApiSecBase, urlParams)

	//post data
	postData := url.Values{}
	postData.Add("method", apiMethod)
	postData.Add("api_key", creds.apikey)

	tmp := make(map[string]string)
	tmp["method"] = apiMethod
	tmp["api_key"] = creds.apikey

	formated, err := formatArgs(args, rules)
	for k, v := range formated {
		tmp[k] = v
		postData.Add(k, v)
	}

	sig := getSignature(tmp, creds.secret)
	postData.Add("api_sig", sig)

	//call API
	res, err := http.PostForm(uri, postData)
	if err != nil {
		return
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	err = parseResponse(body, result)
	return
}