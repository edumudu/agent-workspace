package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// object is a JSON object that keeps its key order, so a merge into the
// user's settings file moves nothing the user wrote.
type object struct {
	members []member
}

type member struct {
	key string
	val json.RawMessage
}

func parse(b []byte) (*object, error) {
	o := &object{}
	if len(bytes.TrimSpace(b)) == 0 {
		return o, nil
	}
	if err := o.UnmarshalJSON(b); err != nil {
		return nil, err
	}
	return o, nil
}

func (o *object) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if tok != json.Delim('{') {
		return errors.New("not a JSON object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, _ := tok.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return err
		}
		o.members = append(o.members, member{key: key, val: val})
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	if _, err := dec.Token(); err == nil {
		return errors.New("trailing data after the JSON object")
	}
	return nil
}

func (o *object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, m := range o.members {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := marshal(m.key)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(m.val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// encode is the settings file format: two-space indent and a final newline.
func (o *object) encode() ([]byte, error) {
	raw, err := o.MarshalJSON()
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func (o *object) get(key string) (json.RawMessage, bool) {
	for _, m := range o.members {
		if m.key == key {
			return m.val, true
		}
	}
	return nil, false
}

func (o *object) set(key string, v any) error {
	val, err := marshal(v)
	if err != nil {
		return err
	}
	for i, m := range o.members {
		if m.key == key {
			o.members[i].val = val
			return nil
		}
	}
	o.members = append(o.members, member{key: key, val: val})
	return nil
}

func (o *object) del(key string) {
	for i, m := range o.members {
		if m.key == key {
			o.members = append(o.members[:i], o.members[i+1:]...)
			return
		}
	}
}

func (o *object) existingObject(key string) (*object, bool, error) {
	raw, ok := o.get(key)
	if !ok {
		return nil, false, nil
	}
	child := &object{}
	if err := child.UnmarshalJSON(raw); err != nil {
		return nil, false, fmt.Errorf("%q: %w", key, err)
	}
	return child, true, nil
}

func (o *object) object(key string) (*object, error) {
	child, ok, err := o.existingObject(key)
	if !ok && err == nil {
		child = &object{}
	}
	return child, err
}

func (o *object) array(key string) ([]json.RawMessage, error) {
	raw, ok := o.get(key)
	if !ok {
		return nil, nil
	}
	var out []json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%q: %w", key, err)
	}
	return out, nil
}

// marshal writes compact JSON without escaping <, > and &, which appear in
// shell commands.
func marshal(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
