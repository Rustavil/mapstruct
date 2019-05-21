// The following directive is necessary to make the package coherent:

// +bкuild ignore

package main

import (
	"reflect"
	"testing"
)

func Test_searchUsedField(t *testing.T) {
	type args struct {
		srcStruct *src
		castRow   string
	}
	tests := []struct {
		name string
		args args
		want *field
	}{
		{
			name: "Simple search",
			args: args{
				srcStruct: &src{
					Alias: "src",
					Fields: []field{
						{
							Name:    "Fields",
							Ptr:     true,
							TypeStr: "int",
						},
						{
							Name:    "Field",
							Ptr:     true,
							TypeStr: "int",
						},
					},
					ShortPath: "",
				},
				castRow: "src.Field",
			},
			want: &field{
				Name:    "Field",
				Ptr:     true,
				TypeStr: "int",
			},
		},
		{
			name: "Simple search",
			args: args{
				srcStruct: &src{
					Alias: "src",
					Fields: []field{
						{
							Name:    "Fields",
							Ptr:     true,
							TypeStr: "int",
						},
						{
							Name:    "Field",
							Ptr:     false,
							TypeStr: "bool",
						},
					},
					ShortPath: "",
				},
				castRow: "src.Field",
			},
			want: &field{
				Name:    "Field",
				Ptr:     false,
				TypeStr: "bool",
			},
		},
		{
			name: "Return nil",
			args: args{
				srcStruct: &src{
					Alias: "src",
					Fields: []field{
						{
							Name:    "Fields",
							Ptr:     true,
							TypeStr: "int",
						},
						{
							Name:    "Field",
							Ptr:     true,
							TypeStr: "int",
						},
					},
					ShortPath: "",
				},
				castRow: "nul",
			},
			want: nil,
		},
		{
			name: "Search by pointer",
			args: args{
				srcStruct: &src{
					Alias: "src",
					Fields: []field{
						{
							Name:    "Fields",
							Ptr:     true,
							TypeStr: "int",
						},
						{
							Name:    "Field",
							Ptr:     true,
							TypeStr: "int",
						},
					},
					ShortPath: "",
				},
				castRow: "*src.Field",
			},
			want: &field{
				Name:    "Field",
				Ptr:     true,
				TypeStr: "int",
			},
		},
		{
			name: "Search by expression",
			args: args{
				srcStruct: &src{
					Alias: "src",
					Fields: []field{
						{
							Name:    "Fields",
							Ptr:     true,
							TypeStr: "int",
						},
						{
							Name:    "Field",
							Ptr:     true,
							TypeStr: "int",
						},
					},
					ShortPath: "",
				},
				castRow: "int64(src.Field)",
			},
			want: &field{
				Name:    "Field",
				Ptr:     true,
				TypeStr: "int",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := searchUsedField(tt.args.srcStruct, tt.args.castRow); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("searchUsedField() = %v, want %v", got, tt.want)
			}
		})
	}
}
