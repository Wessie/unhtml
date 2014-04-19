package unhtml

import (
	"encoding"
	"io"
	"reflect"
	"strconv"
	"strings"

	"gopkg.in/xmlpath.v1"
)

type NoNodesAvailable string

func (e NoNodesAvailable) Error() string {
	return "No nodes found for path: " + string(e)
}

// Error returned if there was an issue with type compatibility
type UnmarshalTypeError struct {
	Value string
	Type  reflect.Type
}

func (e *UnmarshalTypeError) Error() string {
	return "unhtml: cannot unmarshal " + e.Value + " into Go value of type " + e.Type.String()
}

// Error returned if invalid input was given
type InvalidUnmarshalError struct {
	Type reflect.Type
}

func (e *InvalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "unhtml: Unmarshal(nil)"
	}

	if e.Type.Kind() != reflect.Ptr {
		return "unhtml: Unmarshal(non-pointer " + e.Type.String() + ")"
	}

	return "unhtml: Unmarshal(nil " + e.Type.String() + ")"
}

// Unmarshaler is an interface that can be implemented to
// receive the raw resulting node to unmarshal into the
// type.
//
// This receives a []byte of the contents of the current node
// (which might, or might not be HTML depending on node).
type Unmarshaler interface {
	UnmarshalHTML([]byte) error
}

// Decoder
type Decoder struct {
	root *xmlpath.Node
}

// NewDecoder returns a new Decoder by using the contents of the
// io.Reader as HTML input. The io.Reader is consumed whole and
// contents parsed before this function returns.
//
// An error return means something went wrong parsing the HTML.
func NewDecoder(r io.Reader) (*Decoder, error) {
	root, err := xmlpath.ParseHTML(r)

	if err != nil {
		return nil, err
	}

	return &Decoder{root: root}, nil
}

func NewDecoderFromString(s string) (*Decoder, error) {
	return NewDecoder(strings.NewReader(s))
}

// Unmarshal tries to fill the value given with the input previously
// given to the Decoder. See `unhtml.Unmarshal` for full docs.
func (d *Decoder) Unmarshal(res interface{}) error {
	st := &state{}

	return st.unmarshal(d.root, reflect.ValueOf(res))
}

// UnmarshalRelative unmarshals from the node depicted by the path
// given. This allows you to move the root node before unmarshalling.
func (d *Decoder) UnmarshalRelative(path string, res interface{}) error {
	xpath, err := xmlpath.Compile(path)

	if err != nil {
		return err
	}

	st := &state{}

	// Only use the first node we find
	for iter := xpath.Iter(d.root); iter.Next(); {
		return st.unmarshal(iter.Node(), reflect.ValueOf(res))
	}

	return NoNodesAvailable(path)
}

// state is used to keep track of errors that occurred, we don't want
// to return early with an error and leave the unmarshalling in an incomplete
// state.
type state struct {
	firstError error
}

// saveError saves the first error given to saveError and discards all others
func (d *state) saveError(e error) {
	if e != nil && d.firstError == nil {
		d.firstError = e
	}
}

func (d *state) multinode(nodes []*xmlpath.Node, value reflect.Value) error {
	switch value.Kind() {
	case reflect.Array, reflect.Slice:
	default:
		return &UnmarshalTypeError{"Multinode result", value.Type()}
	}

	isSlice := value.Kind() == reflect.Slice

	for i, node := range nodes {
		if isSlice {
			if i >= value.Cap() {
				newcap := value.Cap() + value.Cap()/2
				if newcap < 4 {
					newcap = 4
				}
				newv := reflect.MakeSlice(value.Type(), value.Len(), newcap)
				reflect.Copy(newv, value)
				value.Set(newv)
			}
			if i >= value.Len() {
				value.SetLen(i + 1)
			}
		}

		if i < value.Len() {
			d.saveError(d.unmarshal(node, value.Index(i)))
		}
	}

	return d.firstError
}

func (d *state) unmarshal(root *xmlpath.Node, rv reflect.Value) error {
	m, tm, value := indirect(rv)

	if m != nil {
		return m.UnmarshalHTML(root.Bytes())
	} else if tm != nil {
		return tm.UnmarshalText(root.Bytes())
	}

	if !rv.IsValid() {
		return &InvalidUnmarshalError{nil}
	}

	s := root.String()
	switch value.Kind() {
	case reflect.Struct:
		d.unmarshalStruct(root, value)
	case reflect.Array:
	case reflect.Slice:
		// Short-path for []byte and []rune
		t := value.Type().Elem().Kind()
		if t == reflect.Uint8 {
			value.Set(reflect.ValueOf(root.Bytes()))
		} else if t == reflect.Int32 {
			value.Set(reflect.ValueOf([]rune(s)))
		}
	default:
		// TODO: Change behavior on unsupported type?
	case reflect.Interface:
		if value.NumMethod() != 0 {
			err := &UnmarshalTypeError{s, value.Type()}

			d.saveError(err)
			break
		}

		b := root.Bytes()

		value.Set(reflect.ValueOf(b))
	case reflect.String:
		value.SetString(s)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || value.OverflowInt(n) {
			d.saveError(err)
			break
		}
		value.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil || value.OverflowUint(n) {
			d.saveError(err)
			break
		}
		value.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(s, value.Type().Bits())
		if err != nil || value.OverflowFloat(n) {
			d.saveError(err)
			break
		}
		value.SetFloat(n)

	}

	return d.firstError
}

func (d *state) unmarshalStruct(root *xmlpath.Node, value reflect.Value) error {
	valueType := value.Type()

	if value.Kind() != reflect.Struct {
		return &InvalidUnmarshalError{
			Type: value.Type(),
		}
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		structField := valueType.Field(i)

		// Find the struct tag if any
		path := structField.Tag.Get("unhtml")

		if path == "" {
			// Skip fields with no tag, since we require an xpath
			continue
		}

		if !field.CanSet() {
			// TODO: Some way to feedback to the user
			continue
		}

		var (
			nodes = make([]*xmlpath.Node, 0, 12)
			xpath = xmlpath.MustCompile(path)
		)

		for iter := xpath.Iter(root); iter.Next(); {
			node := iter.Node()

			nodes = append(nodes, node)
		}

		if len(nodes) > 1 {
			d.multinode(nodes, field)
			continue
		} else if len(nodes) < 1 {
			continue
		}

		d.unmarshal(nodes[0], field)
	}

	return d.firstError
}

// indirect walks down v allocating pointers as needed until it gets to a non-pointer
//
// indirect original can be found in the stdlib encoding/json, credit to Go authors
func indirect(v reflect.Value) (Unmarshaler, encoding.TextUnmarshaler, reflect.Value) {
	if v.Kind() != reflect.Ptr && v.Type().Name() != "" && v.CanAddr() {
		v = v.Addr()
	}

	for {
		if v.Kind() == reflect.Interface && !v.IsNil() {
			e := v.Elem()
			if e.Kind() == reflect.Ptr && !e.IsNil() && e.Elem().Kind() == reflect.Ptr {
				v = e
				continue
			}
		}

		if v.Kind() != reflect.Ptr {
			break
		}

		if v.Elem().Kind() != reflect.Ptr && v.CanSet() {
			break
		}

		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		if v.Type().NumMethod() > 0 {
			if u, ok := v.Interface().(Unmarshaler); ok {
				return u, nil, reflect.Value{}
			}

			if u, ok := v.Interface().(encoding.TextUnmarshaler); ok {
				return nil, u, reflect.Value{}
			}
		}
		v = v.Elem()
	}
	return nil, nil, v
}
