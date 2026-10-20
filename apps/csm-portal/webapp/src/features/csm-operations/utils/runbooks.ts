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
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

export interface Runbook {
  id: string;
  name: string;
  /** Null when the runbook has not been written yet. */
  runbookUrl: string | null;
  /** Alert titles; an incident whose subject contains one gets this runbook. */
  alerts: string[];
  createdOn: string;
  createdBy: string;
  updatedOn: string;
  updatedBy: string;
}

export interface RunbookInput {
  name: string;
  runbookUrl: string | null;
  alerts: string[];
}

export type RunbookCoverage = "all" | "linked" | "missing";

export function isValidRunbookUrl(value: string): boolean {
  if (!value.trim()) return true;
  try {
    const url = new URL(value.trim());
    return url.protocol === "https:" || url.protocol === "http:";
  } catch {
    return false;
  }
}

/** Trimmed alert names with blanks and repeats (ignoring case) dropped, in their first order. */
export function cleanAlertNames(names: string[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const raw of names) {
    const name = raw.trim();
    const key = name.toLowerCase();
    if (!name || seen.has(key)) continue;
    seen.add(key);
    out.push(name);
  }
  return out;
}

export interface RunbookMatch<T extends Pick<Runbook, "name" | "runbookUrl" | "alerts">> {
  runbook: T;
  alertName: string;
}

/** The runbook for an incident: the one holding the longest alert name its subject contains, case-insensitively. Mirrors entity-service. */
export function findRunbookForIncident<T extends Pick<Runbook, "name" | "runbookUrl" | "alerts">>(
  runbooks: T[],
  subject: string,
): RunbookMatch<T> | undefined {
  const haystack = subject.toLowerCase();
  let best: RunbookMatch<T> | undefined;
  let bestLen = 0;
  for (const runbook of runbooks) {
    for (const alertName of runbook.alerts) {
      const needle = alertName.trim().toLowerCase();
      if (needle && needle.length > bestLen && haystack.includes(needle)) {
        best = { runbook, alertName };
        bestLen = needle.length;
      }
    }
  }
  return best;
}

/**
 * Reads an import file: a JSON object whose keys are comma-separated alert names
 * and whose values are links ("" for none), named after the first alert; or an
 * array of {name, runbookUrl, alerts}, where alerts is a list or a comma-separated
 * string (an older {alertName, runbookUrl} entry is read as one alert). Throws with
 * a readable message otherwise.
 */
export function parseRunbookImport(text: string): RunbookInput[] {
  let data: unknown;
  try {
    data = JSON.parse(text);
  } catch {
    throw new Error("The file is not valid JSON.");
  }
  const toInput = (name: unknown, alerts: unknown, url: unknown, label: string): RunbookInput => {
    const alertList = cleanAlertNames(
      typeof alerts === "string" ? alerts.split(",") : Array.isArray(alerts) ? alerts.map((a) => (typeof a === "string" ? a : "")) : [],
    );
    if (alertList.length === 0) {
      throw new Error(`"${label}" needs at least one alert name.`);
    }
    const runbookName = typeof name === "string" && name.trim() ? name.trim() : alertList[0];
    if (url !== null && url !== undefined && typeof url !== "string") {
      throw new Error(`The link for "${runbookName}" is not text.`);
    }
    const link = typeof url === "string" ? url.trim() : "";
    if (!isValidRunbookUrl(link)) {
      throw new Error(`The link for "${runbookName}" is not a full http(s) link.`);
    }
    return { name: runbookName, runbookUrl: link || null, alerts: alertList };
  };
  if (Array.isArray(data)) {
    return data.map((row, i) => {
      const r = (row ?? {}) as Record<string, unknown>;
      return toInput(r.name, r.alerts ?? r.alertName, r.runbookUrl, typeof r.name === "string" && r.name.trim() ? r.name.trim() : `Entry ${i + 1}`);
    });
  }
  if (data && typeof data === "object") {
    return Object.entries(data as Record<string, unknown>).map(([key, url]) => toInput(undefined, key, url, key));
  }
  throw new Error("Expected an object of alert names to link, or a list of runbooks.");
}
