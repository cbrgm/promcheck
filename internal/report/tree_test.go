package report

import (
	"reflect"
	"testing"
)

func Test_newNode(t *testing.T) {
	tests := []struct {
		name string
		text string
		want Tree
	}{
		{
			name: "must create node",
			text: "node-foo",
			want: &node{
				text:  "node-foo",
				nodes: []Tree{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newNode(tt.text); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newNode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_node_AddNode(t1 *testing.T) {
	type fields struct {
		text  string
		nodes []Tree
	}
	type args struct {
		text string
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		want       Tree
		wantLength int
	}{
		{
			name: "must add node",
			fields: fields{
				text:  "foo",
				nodes: []Tree{},
			},
			args: args{
				text: "bar",
			},
			want: &node{
				text:  "bar",
				nodes: []Tree{},
			},
			wantLength: 1,
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &node{
				text:  tt.fields.text,
				nodes: tt.fields.nodes,
			}
			if got := t.AddNode(tt.args.text); !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("AddNode() = %v, want %v", got, tt.want)
				t1.Errorf("AddNode() = %v, want %v", len(got.Nodes()), tt.wantLength)
			}
		})
	}
}

func Test_node_AddSubtree(t1 *testing.T) {
	type fields struct {
		text  string
		nodes []Tree
	}
	type args struct {
		tree Tree
	}
	tests := []struct {
		name   string
		fields fields
		args   args
	}{
		{
			name: "must set subtree to empty tree",
			fields: fields{
				text:  "bar",
				nodes: []Tree{},
			},
			args: args{
				tree: newNode("foobar"),
			},
		},
		{
			name: "add subtree to existing tree",
			fields: fields{
				text: "foo",
				nodes: []Tree{
					newNode("foo"),
					newNode("bar"),
				},
			},
			args: args{
				tree: newNode("foobar"),
			},
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &node{
				text:  tt.fields.text,
				nodes: tt.fields.nodes,
			}
			t.AddSubtree(tt.args.tree)
		})
	}
}

func Test_node_Nodes(t1 *testing.T) {
	type fields struct {
		text  string
		nodes []Tree
	}
	tests := []struct {
		name   string
		fields fields
		want   []Tree
	}{
		{
			name: "must get nodes",
			fields: fields{
				text: "root",
				nodes: []Tree{
					newNode("foo"),
					newNode("bar"),
				},
			},
			want: []Tree{
				newNode("foo"),
				newNode("bar"),
			},
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &node{
				text:  tt.fields.text,
				nodes: tt.fields.nodes,
			}
			if got := t.Nodes(); !reflect.DeepEqual(got, tt.want) {
				t1.Errorf("Nodes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_node_Print(t1 *testing.T) {
	type fields struct {
		text  string
		nodes []Tree
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "must print empty tree",
			fields: fields{
				text:  "root",
				nodes: []Tree{},
			},
			want: "root\n",
		},
		{
			name: "must print tree",
			fields: fields{
				text: "root",
				nodes: []Tree{
					newNode("foo"),
					newNode("bar"),
				},
			},
			want: `root
├── foo
└── bar
`,
		},
		{
			name: "must print one level tree",
			fields: fields{
				text: "root",
				nodes: []Tree{
					newNode("foo"),
					&node{
						text: "level-one",
						nodes: []Tree{
							newNode("foo"),
							newNode("bar"),
						},
					},
				},
			},
			want: `root
├── foo
└── level-one
    ├── foo
    └── bar
`,
		},
		{
			name: "must print two levels tree",
			fields: fields{
				text: "root",
				nodes: []Tree{
					newNode("foo"),
					&node{
						text: "level-one",
						nodes: []Tree{
							newNode("foo"),
							newNode("bar"),
							&node{
								text: "level-two",
								nodes: []Tree{
									newNode("foo"),
									newNode("bar"),
								},
							},
						},
					},
				},
			},
			want: `root
├── foo
└── level-one
    ├── foo
    ├── bar
    └── level-two
        ├── foo
        └── bar
`,
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &node{
				text:  tt.fields.text,
				nodes: tt.fields.nodes,
			}
			if got := t.Print(); got != tt.want {
				t1.Errorf("Print() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_node_String(t1 *testing.T) {
	type fields struct {
		text  string
		nodes []Tree
	}
	tests := []struct {
		name   string
		fields fields
		want   string
	}{
		{
			name: "must print text",
			fields: fields{
				text:  "foo",
				nodes: []Tree{},
			},
			want: "foo",
		},
		{
			name: "must print empty text",
			fields: fields{
				text:  "",
				nodes: []Tree{},
			},
			want: "",
		},
	}
	for _, tt := range tests {
		t1.Run(tt.name, func(t1 *testing.T) {
			t := &node{
				text:  tt.fields.text,
				nodes: tt.fields.nodes,
			}
			if got := t.String(); got != tt.want {
				t1.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
