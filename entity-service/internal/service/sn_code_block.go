// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

package service

import "regexp"

// ServiceNow HTML-escapes anything written to a work_notes/comments journal
// field by default, unless the value is wrapped in "[code]"/"[/code]" -- its
// documented opt-in signal to render the content as HTML instead of showing
// the tags literally. apps/csm-portal/backend's direct ServiceNow client
// already relies on this for case work notes (see
// internal/servicenow/worknotes.go); incident work notes/comments need the
// same wrap, since they go HTML-sourced but through this service's Choreo
// ServiceNow integration instead.
var (
	codeBlockOpenRe  = regexp.MustCompile(`\[code\]`)
	codeBlockCloseRe = regexp.MustCompile(`\[/code\]`)
)

// wrapCodeBlock marks an HTML-sourced work note/comment so ServiceNow renders
// it instead of displaying the tags literally. nil is passed through as nil.
func wrapCodeBlock(value *string) *string {
	if value == nil {
		return nil
	}
	wrapped := "[code]" + *value + "[/code]"
	return &wrapped
}

// trimCodeBlock strips ServiceNow's "[code]"/"[/code]" wrapper markers from a
// work note/comment read back from ServiceNow -- mirrors
// apps/csm-portal/backend/internal/servicenow/cases.go's trimCodeBlock.
// nil is passed through as nil.
func trimCodeBlock(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := codeBlockCloseRe.ReplaceAllString(codeBlockOpenRe.ReplaceAllString(*value, ""), "")
	return &trimmed
}
