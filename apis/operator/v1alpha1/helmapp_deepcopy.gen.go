// Code generated by protoc-gen-deepcopy. DO NOT EDIT.
package v1alpha1

import (
	proto "github.com/golang/protobuf/proto"
)

// DeepCopyInto supports using HelmAppSpec within kubernetes types, where deepcopy-gen is used.
func (in *HelmAppSpec) DeepCopyInto(out *HelmAppSpec) {
	p := proto.Clone(in).(*HelmAppSpec)
	*out = *p
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HelmAppSpec. Required by controller-gen.
func (in *HelmAppSpec) DeepCopy() *HelmAppSpec {
	if in == nil {
		return nil
	}
	out := new(HelmAppSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInterface is an autogenerated deepcopy function, copying the receiver, creating a new HelmAppSpec. Required by controller-gen.
func (in *HelmAppSpec) DeepCopyInterface() interface{} {
	return in.DeepCopy()
}

// DeepCopyInto supports using HelmComponent within kubernetes types, where deepcopy-gen is used.
func (in *HelmComponent) DeepCopyInto(out *HelmComponent) {
	p := proto.Clone(in).(*HelmComponent)
	*out = *p
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HelmComponent. Required by controller-gen.
func (in *HelmComponent) DeepCopy() *HelmComponent {
	if in == nil {
		return nil
	}
	out := new(HelmComponent)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInterface is an autogenerated deepcopy function, copying the receiver, creating a new HelmComponent. Required by controller-gen.
func (in *HelmComponent) DeepCopyInterface() interface{} {
	return in.DeepCopy()
}

// DeepCopyInto supports using HelmRepo within kubernetes types, where deepcopy-gen is used.
func (in *HelmRepo) DeepCopyInto(out *HelmRepo) {
	p := proto.Clone(in).(*HelmRepo)
	*out = *p
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HelmRepo. Required by controller-gen.
func (in *HelmRepo) DeepCopy() *HelmRepo {
	if in == nil {
		return nil
	}
	out := new(HelmRepo)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInterface is an autogenerated deepcopy function, copying the receiver, creating a new HelmRepo. Required by controller-gen.
func (in *HelmRepo) DeepCopyInterface() interface{} {
	return in.DeepCopy()
}

// DeepCopyInto supports using HelmAppStatus within kubernetes types, where deepcopy-gen is used.
func (in *HelmAppStatus) DeepCopyInto(out *HelmAppStatus) {
	p := proto.Clone(in).(*HelmAppStatus)
	*out = *p
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HelmAppStatus. Required by controller-gen.
func (in *HelmAppStatus) DeepCopy() *HelmAppStatus {
	if in == nil {
		return nil
	}
	out := new(HelmAppStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInterface is an autogenerated deepcopy function, copying the receiver, creating a new HelmAppStatus. Required by controller-gen.
func (in *HelmAppStatus) DeepCopyInterface() interface{} {
	return in.DeepCopy()
}

// DeepCopyInto supports using HelmComponentStatus within kubernetes types, where deepcopy-gen is used.
func (in *HelmComponentStatus) DeepCopyInto(out *HelmComponentStatus) {
	p := proto.Clone(in).(*HelmComponentStatus)
	*out = *p
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HelmComponentStatus. Required by controller-gen.
func (in *HelmComponentStatus) DeepCopy() *HelmComponentStatus {
	if in == nil {
		return nil
	}
	out := new(HelmComponentStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInterface is an autogenerated deepcopy function, copying the receiver, creating a new HelmComponentStatus. Required by controller-gen.
func (in *HelmComponentStatus) DeepCopyInterface() interface{} {
	return in.DeepCopy()
}

// DeepCopyInto supports using HelmResourceStatus within kubernetes types, where deepcopy-gen is used.
func (in *HelmResourceStatus) DeepCopyInto(out *HelmResourceStatus) {
	p := proto.Clone(in).(*HelmResourceStatus)
	*out = *p
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new HelmResourceStatus. Required by controller-gen.
func (in *HelmResourceStatus) DeepCopy() *HelmResourceStatus {
	if in == nil {
		return nil
	}
	out := new(HelmResourceStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInterface is an autogenerated deepcopy function, copying the receiver, creating a new HelmResourceStatus. Required by controller-gen.
func (in *HelmResourceStatus) DeepCopyInterface() interface{} {
	return in.DeepCopy()
}