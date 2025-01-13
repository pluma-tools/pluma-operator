// Code generated by protoc-gen-jsonshim. DO NOT EDIT.
package v1alpha1

import (
	bytes "bytes"

	jsonpb "github.com/golang/protobuf/jsonpb"
)

// MarshalJSON is a custom marshaler for IstioOperatorSpec
func (this *IstioOperatorSpec) MarshalJSON() ([]byte, error) {
	str, err := OperatorMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for IstioOperatorSpec
func (this *IstioOperatorSpec) UnmarshalJSON(b []byte) error {
	return OperatorUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

// MarshalJSON is a custom marshaler for InstallStatus
func (this *InstallStatus) MarshalJSON() ([]byte, error) {
	str, err := OperatorMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for InstallStatus
func (this *InstallStatus) UnmarshalJSON(b []byte) error {
	return OperatorUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

// MarshalJSON is a custom marshaler for InstallStatus_VersionStatus
func (this *InstallStatus_VersionStatus) MarshalJSON() ([]byte, error) {
	str, err := OperatorMarshaler.MarshalToString(this)
	return []byte(str), err
}

// UnmarshalJSON is a custom unmarshaler for InstallStatus_VersionStatus
func (this *InstallStatus_VersionStatus) UnmarshalJSON(b []byte) error {
	return OperatorUnmarshaler.Unmarshal(bytes.NewReader(b), this)
}

var (
	OperatorMarshaler   = &jsonpb.Marshaler{}
	OperatorUnmarshaler = &jsonpb.Unmarshaler{AllowUnknownFields: true}
)
