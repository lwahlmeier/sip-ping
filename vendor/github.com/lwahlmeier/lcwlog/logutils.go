package lcwlog

import (
	"fmt"
	"strings"
)

func simpleFormatter1(msg string, args ...interface{}) string {
	sb := strings.Builder{}
	subs := strings.Split(msg, "{}")

	for i, v := range subs {
		v = strings.Replace(strings.Replace(v, "{{", "{", -1), "}}", "}", -1)
		sb.WriteString(v)
		if i < len(args) {
			sb.WriteString(fmt.Sprintf("%v", args[i]))
		}
	}
	return sb.String()
}

func simpleFormatter2(msg string, args ...interface{}) string {
	sb := strings.Builder{}
	subs := strings.Split(msg, "{}")

	for i, v := range subs {
		sb.WriteString(v)
		if i < len(args) {
			sb.WriteString(fmt.Sprintf("%v", args[i]))
		}
	}
	tmp := sb.String()
	tmp = strings.Replace(strings.Replace(tmp, "{{", "{", -1), "}}", "}", -1)
	return tmp
}

func argFormatter1(msg string, args ...interface{}) string {

	newStr := msg
	for i, v := range args {
		v2 := fmt.Sprintf("%v", v)
		newStr = strings.Replace(newStr, "{}", v2, 1)
		newStr = strings.Replace(newStr, fmt.Sprintf("{%d}", i), v2, -1)
	}
	newStr = strings.Replace(strings.Replace(newStr, "{{", "{", -1), "}}", "}", -1)

	return newStr
}
