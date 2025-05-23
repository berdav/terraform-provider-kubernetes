// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package kubernetes

import (
	"encoding/base64"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/robfig/cron"
	"k8s.io/apimachinery/pkg/api/resource"
	apiValidation "k8s.io/apimachinery/pkg/api/validation"
	utilValidation "k8s.io/apimachinery/pkg/util/validation"
)

func validateAnnotations(value interface{}, key string) (ws []string, es []error) {
	m := value.(map[string]interface{})
	for k := range m {
		errors := utilValidation.IsQualifiedName(strings.ToLower(k))
		if len(errors) > 0 {
			for _, e := range errors {
				es = append(es, fmt.Errorf("%s (%q) %s", key, k, e))
			}
		}
	}
	return
}

func validateBase64Encoded(v interface{}, key string) (ws []string, es []error) {
	s, ok := v.(string)
	if !ok {
		es = []error{fmt.Errorf("%s: must be a non-nil base64-encoded string", key)}
		return
	}

	_, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		es = []error{fmt.Errorf("%s: must be a base64-encoded string", key)}
		return
	}
	return
}

func validateBase64EncodedMap(value interface{}, key string) (ws []string, es []error) {
	m, ok := value.(map[string]interface{})
	if !ok {
		es = []error{fmt.Errorf("%s: must be a map of strings to base64 encoded strings", key)}
		return
	}

	for k, v := range m {
		_, errs := validateBase64Encoded(v, k)
		for _, e := range errs {
			es = append(es, fmt.Errorf("%s (%q) %s", k, v, e))
		}
	}

	return
}

func validateName(value interface{}, key string) (ws []string, es []error) {
	v := value.(string)
	errors := apiValidation.NameIsDNSSubdomain(v, false)
	if len(errors) > 0 {
		for _, err := range errors {
			es = append(es, fmt.Errorf("%s %s", key, err))
		}
	}
	return
}

func validateGenerateName(value interface{}, key string) (ws []string, es []error) {
	v := value.(string)

	errors := apiValidation.NameIsDNSLabel(v, true)
	if len(errors) > 0 {
		for _, err := range errors {
			es = append(es, fmt.Errorf("%s %s", key, err))
		}
	}
	return
}

func validateLabels(value interface{}, key string) (ws []string, es []error) {
	m := value.(map[string]interface{})
	for k, v := range m {
		for _, msg := range utilValidation.IsQualifiedName(k) {
			es = append(es, fmt.Errorf("%s (%q) %s", key, k, msg))
		}
		val, isString := v.(string)
		if !isString {
			es = append(es, fmt.Errorf("%s.%s (%#v): Expected value to be string", key, k, v))
			return
		}
		for _, msg := range utilValidation.IsValidLabelValue(val) {
			es = append(es, fmt.Errorf("%s (%q) %s", key, val, msg))
		}
	}
	return
}

func validatePortNum(value interface{}, key string) (ws []string, es []error) {
	errors := utilValidation.IsValidPortNum(value.(int))
	if len(errors) > 0 {
		for _, err := range errors {
			es = append(es, fmt.Errorf("%s %s", key, err))
		}
	}
	return
}

func validatePortName(value interface{}, key string) (ws []string, es []error) {
	errors := utilValidation.IsValidPortName(value.(string))
	if len(errors) > 0 {
		for _, err := range errors {
			es = append(es, fmt.Errorf("%s %s", key, err))
		}
	}
	return
}
func validatePortNumOrName(value interface{}, key string) (ws []string, es []error) {
	switch t := value.(type) {
	case string:
		intVal, err := strconv.Atoi(t)
		if err != nil {
			return validatePortName(value, key)
		}
		return validatePortNum(intVal, key)
	case int:
		return validatePortNum(value, key)

	default:
		es = append(es, fmt.Errorf("%s must be defined of type string or int on the schema", key))
		return
	}
}

func validateResourceList(value interface{}, key string) (ws []string, es []error) {
	m := value.(map[string]interface{})
	for k, value := range m {
		if _, ok := value.(int); ok {
			continue
		}

		if v, ok := value.(string); ok {
			_, err := resource.ParseQuantity(v)
			if err != nil {
				es = append(es, fmt.Errorf("%s.%s (%q): %s", key, k, v, err))
			}
			continue
		}

		err := "Value can be either string or int"
		es = append(es, fmt.Errorf("%s.%s (%#v): %s", key, k, value, err))
	}
	return
}

func validateResourceQuantity(value interface{}, key string) (ws []string, es []error) {
	if v, ok := value.(string); ok {
		_, err := resource.ParseQuantity(v)
		if err != nil {
			es = append(es, fmt.Errorf("%s.%s : %s", key, v, err))
		}
	}
	return
}

func validateNonNegativeInteger(value interface{}, key string) (ws []string, es []error) {
	v := value.(int)
	if v < 0 {
		es = append(es, fmt.Errorf("%s must be greater than or equal to 0", key))
	}
	return
}

func validatePositiveInteger(value interface{}, key string) (ws []string, es []error) {
	v := value.(int)
	if v <= 0 {
		es = append(es, fmt.Errorf("%s must be greater than 0", key))
	}
	return
}

func validateTerminationGracePeriodSeconds(value interface{}, key string) (ws []string, es []error) {
	v := value.(int)
	if v < 0 {
		es = append(es, fmt.Errorf("%s must be greater than or equal to 0", key))
	}
	return
}

func validateIntGreaterThan(minValue int) func(value interface{}, key string) (ws []string, es []error) {
	return func(value interface{}, key string) (ws []string, es []error) {
		v := value.(int)
		if v < minValue {
			es = append(es, fmt.Errorf("%s must be greater than or equal to %d", key, minValue))
		}
		return
	}
}

// validateTypeStringNullableInt provides custom error messaging for TypeString ints
// Some arguments require an int value or unspecified, empty field.
func validateTypeStringNullableInt(v interface{}, k string) (ws []string, es []error) {
	value, ok := v.(string)
	if !ok {
		es = append(es, fmt.Errorf("expected type of %s to be string", k))
		return
	}

	if value == "" {
		return
	}

	if _, err := strconv.ParseInt(value, 10, 64); err != nil {
		es = append(es, fmt.Errorf("%s: cannot parse '%s' as int: %s", k, value, err))
	}

	return
}

func validateModeBits(value interface{}, key string) (ws []string, es []error) {
	if !strings.HasPrefix(value.(string), "0") {
		es = append(es, fmt.Errorf("%s: value %s should start with '0' (octal numeral)", key, value.(string)))
	}
	v, err := strconv.ParseInt(value.(string), 8, 32)
	if err != nil {
		es = append(es, fmt.Errorf("%s :Cannot parse octal numeral (%#v): %s", key, value, err))
	}
	if v < 0 || v > 0777 {
		es = append(es, fmt.Errorf("%s (%#o) expects octal notation (a value between 0 and 0777)", key, v))
	}
	return
}

// validatePath makes sure path:
//   - is not abs path
//   - does not contain any '..' elements
//   - does not start with '..'
func validatePath(v interface{}, k string) ([]string, []error) {
	// inherit logic from the Kubernetes API validation: https://github.com/kubernetes/kubernetes/blob/master/pkg/apis/core/validation/validation.go
	targetPath := v.(string)

	if path.IsAbs(targetPath) {
		return []string{}, []error{errors.New("must be a relative path")}
	}

	parts := strings.Split(filepath.ToSlash(targetPath), "/")
	for _, part := range parts {
		if part == ".." {
			return []string{}, []error{fmt.Errorf("%q must not contain %q", k, "..")}
		}
	}

	if strings.HasPrefix(targetPath, "..") {
		return []string{}, []error{fmt.Errorf("%q must not start with %q", k, "..")}
	}

	return []string{}, []error{}
}

func validateTypeStringNullableIntOrPercent(v interface{}, key string) (ws []string, es []error) {
	value, ok := v.(string)
	if !ok {
		es = append(es, fmt.Errorf("expected type of %s to be string", key))
		return
	}

	if value == "" {
		return
	}

	if strings.HasSuffix(value, "%") {
		percent, err := strconv.ParseInt(strings.TrimSuffix(value, "%"), 10, 32)
		if err != nil {
			es = append(es, fmt.Errorf("%s: cannot parse '%s' as percent: %s", key, value, err))
		}
		if percent < 0 || percent > 100 {
			es = append(es, fmt.Errorf("%s: '%s' is not between 0%% and 100%%", key, value))
		}
	} else if _, err := strconv.ParseInt(value, 10, 32); err != nil {
		es = append(es, fmt.Errorf("%s: cannot parse '%s' as int or percent: %s", key, value, err))
	}

	return
}

func validateCronExpression(v interface{}, k string) ([]string, []error) {
	errors := make([]error, 0)

	_, err := cron.ParseStandard(v.(string))
	if err != nil {
		errors = append(errors, fmt.Errorf("%q should be a valid Cron expression", k))
	}

	return []string{}, errors
}
