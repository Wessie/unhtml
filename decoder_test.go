package unhtml

import (
	"reflect"
	"strings"
	"testing"
)

type Test struct {
	path     string
	html     string
	expected interface{}
}

var relativeTests = []Test{
	{
		path:     "/test",
		html:     "<test>-555</test>",
		expected: int(-555),
	},
	{
		path:     "/test",
		html:     "<test>444</test>",
		expected: uint(444),
	},
	{
		path:     "/test",
		html:     "<test>林原め</test>",
		expected: []rune(`林原め`),
	},
	{
		path:     "/test",
		html:     "<test>Hello World</test>",
		expected: "Hello World",
	},
	{
		path:     "/test",
		html:     "<test><inner>Hello</inner> World</test>",
		expected: "Hello World",
	},
	{
		path:     "/test",
		html:     "<test>Hello World</test>",
		expected: []byte(`Hello World`),
	},
	{
		path: "/test",
		html: "<test><div>Hello</div><span>World</span></test>",
		expected: struct {
			A string `unhtml:"div"`
			B string `unhtml:"span"`
		}{"Hello", "World"},
	},
	{
		path:     "/ul",
		html:     "<ul><li>0</li><li>1</li><li>2</li></ul>",
		expected: []int{0, 1, 2},
	},
}

func TestUnmarshalRelative(t *testing.T) {
	for _, test := range relativeTests {
		r := strings.NewReader(test.html)

		d, err := NewDecoder(r)

		if err != nil {
			t.Errorf("Failed parsing html: %s", err)
			continue
		}

		// Create a variable to hold the result in
		input := reflect.New(reflect.TypeOf(test.expected))

		if err := d.UnmarshalRelative(test.path, input.Interface()); err != nil {
			t.Errorf("Failed unmarshalling: %s", err)
			continue
		}

		// Grab the not-pointer-result
		result := input.Elem().Interface()

		if !reflect.DeepEqual(result, test.expected) {
			t.Errorf("Expectation fail: %v != %v", result, test.expected)
		}
	}
}
