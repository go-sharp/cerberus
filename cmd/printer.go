package main

import (
	"fmt"
	"io"
	"strings"
)

type keyValuePrinter struct {
	ci         int
	indentSize int
	items      []struct {
		indent int
		key    string
		value  interface{}
	}
	maxKeys map[int]int
}

func (p *keyValuePrinter) unindent() {
	if p.ci > 0 {
		p.ci--
	}
}

func (p *keyValuePrinter) indent() {
	p.ci++
}

func (p *keyValuePrinter) println(key string, value interface{}) {
	p.updateKey(key)
	p.items = append(p.items, struct {
		indent int
		key    string
		value  interface{}
	}{
		indent: p.ci,
		key:    key,
		value:  value,
	})
}

func (p *keyValuePrinter) updateKey(key string) {
	if p.maxKeys == nil {
		p.maxKeys = map[int]int{}
	}

	n := len([]rune(key))
	if k, ok := p.maxKeys[p.ci]; ok && n <= k {
		return
	}
	p.maxKeys[p.ci] = n
}

func (p *keyValuePrinter) writeTo(writer io.Writer) {
	for _, item := range p.items {
		fmt.Fprintf(writer, strings.Repeat(" ", item.indent*p.indentSize))

		n := p.maxKeys[item.indent] - len([]rune(item.key))
		if n < 0 {
			n = 0
		}

		if item.value == nil {
			fmt.Fprintf(writer, "%v\n", item.key)
			continue
		}
		fmt.Fprintf(writer, "%v : %v\n", item.key+strings.Repeat(" ", n), item.value)
	}

	p.ci = 0
	p.items = p.items[:0]
}
