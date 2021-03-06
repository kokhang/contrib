/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"fmt"
	"reflect"

	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/unversioned"
	v1 "k8s.io/kubernetes/pkg/api/v1"
	"k8s.io/kubernetes/pkg/apis/batch"
	"k8s.io/kubernetes/pkg/conversion"
	"k8s.io/kubernetes/pkg/runtime"
)

func addConversionFuncs(scheme *runtime.Scheme) {
	// Add non-generated conversion functions
	err := scheme.AddConversionFuncs(
		Convert_batch_JobSpec_To_v1_JobSpec,
		Convert_v1_JobSpec_To_batch_JobSpec,
	)
	if err != nil {
		// If one of the conversion functions is malformed, detect it immediately.
		panic(err)
	}

	err = api.Scheme.AddFieldLabelConversionFunc("batch/v1", "Job",
		func(label, value string) (string, string, error) {
			switch label {
			case "metadata.name", "metadata.namespace", "status.successful":
				return label, value, nil
			default:
				return "", "", fmt.Errorf("field label not supported: %s", label)
			}
		})
	if err != nil {
		// If one of the conversion functions is malformed, detect it immediately.
		panic(err)
	}
}

func Convert_batch_JobSpec_To_v1_JobSpec(in *batch.JobSpec, out *JobSpec, s conversion.Scope) error {
	if defaulting, found := s.DefaultingInterface(reflect.TypeOf(*in)); found {
		defaulting.(func(*batch.JobSpec))(in)
	}
	if in.Parallelism != nil {
		out.Parallelism = new(int32)
		*out.Parallelism = int32(*in.Parallelism)
	} else {
		out.Parallelism = nil
	}
	if in.Completions != nil {
		out.Completions = new(int32)
		*out.Completions = int32(*in.Completions)
	} else {
		out.Completions = nil
	}
	if in.ActiveDeadlineSeconds != nil {
		out.ActiveDeadlineSeconds = new(int64)
		*out.ActiveDeadlineSeconds = *in.ActiveDeadlineSeconds
	} else {
		out.ActiveDeadlineSeconds = nil
	}
	// unable to generate simple pointer conversion for unversioned.LabelSelector -> v1.LabelSelector
	if in.Selector != nil {
		out.Selector = new(LabelSelector)
		if err := Convert_unversioned_LabelSelector_To_v1_LabelSelector(in.Selector, out.Selector, s); err != nil {
			return err
		}
	} else {
		out.Selector = nil
	}
	if in.ManualSelector != nil {
		out.ManualSelector = new(bool)
		*out.ManualSelector = *in.ManualSelector
	} else {
		out.ManualSelector = nil
	}

	if err := v1.Convert_api_PodTemplateSpec_To_v1_PodTemplateSpec(&in.Template, &out.Template, s); err != nil {
		return err
	}
	return nil
}

func Convert_v1_JobSpec_To_batch_JobSpec(in *JobSpec, out *batch.JobSpec, s conversion.Scope) error {
	if defaulting, found := s.DefaultingInterface(reflect.TypeOf(*in)); found {
		defaulting.(func(*JobSpec))(in)
	}
	if in.Parallelism != nil {
		out.Parallelism = new(int)
		*out.Parallelism = int(*in.Parallelism)
	} else {
		out.Parallelism = nil
	}
	if in.Completions != nil {
		out.Completions = new(int)
		*out.Completions = int(*in.Completions)
	} else {
		out.Completions = nil
	}
	if in.ActiveDeadlineSeconds != nil {
		out.ActiveDeadlineSeconds = new(int64)
		*out.ActiveDeadlineSeconds = *in.ActiveDeadlineSeconds
	} else {
		out.ActiveDeadlineSeconds = nil
	}
	// unable to generate simple pointer conversion for v1.LabelSelector -> unversioned.LabelSelector
	if in.Selector != nil {
		out.Selector = new(unversioned.LabelSelector)
		if err := Convert_v1_LabelSelector_To_unversioned_LabelSelector(in.Selector, out.Selector, s); err != nil {
			return err
		}
	} else {
		out.Selector = nil
	}
	if in.ManualSelector != nil {
		out.ManualSelector = new(bool)
		*out.ManualSelector = *in.ManualSelector
	} else {
		out.ManualSelector = nil
	}

	if err := v1.Convert_v1_PodTemplateSpec_To_api_PodTemplateSpec(&in.Template, &out.Template, s); err != nil {
		return err
	}
	return nil
}
