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

import { describe, expect, it } from "vitest";
import { cleanAlertNames, findRunbookForIncident, parseRunbookImport } from "./runbooks";

describe("parseRunbookImport", () => {
  it("reads an object whose keys are comma-separated alert names, keeping spaces inside a name", () => {
    expect(
      parseRunbookImport('{"Alert 1, Alert 2 ,alert 1": "https://docs.example.com/a", " Alert B ": ""}'),
    ).toEqual([
      { name: "Alert 1", runbookUrl: "https://docs.example.com/a", alerts: ["Alert 1", "Alert 2"] },
      { name: "Alert B", runbookUrl: null, alerts: ["Alert B"] },
    ]);
  });

  it("reads a list of runbooks with a list or a comma-separated string of alerts", () => {
    expect(
      parseRunbookImport('[{"name": "Gateway", "runbookUrl": null, "alerts": ["a", "b"]}, {"alerts": "c, d"}]'),
    ).toEqual([
      { name: "Gateway", runbookUrl: null, alerts: ["a", "b"] },
      { name: "c", runbookUrl: null, alerts: ["c", "d"] },
    ]);
  });

  it("still reads an older list entry with one alertName", () => {
    expect(parseRunbookImport('[{"alertName": "A", "runbookUrl": null}]')).toEqual([
      { name: "A", runbookUrl: null, alerts: ["A"] },
    ]);
  });

  it("refuses bad JSON, entries with no alert and links that are not http(s)", () => {
    expect(() => parseRunbookImport("{")).toThrow("not valid JSON");
    expect(() => parseRunbookImport('[{"name": "N", "alerts": [" "]}]')).toThrow("at least one alert name");
    expect(() => parseRunbookImport('{" , ": "https://x.example"}')).toThrow("at least one alert name");
    expect(() => parseRunbookImport('{"A": "javascript:alert(1)"}')).toThrow("http(s)");
  });
});

describe("cleanAlertNames", () => {
  it("trims and drops blanks and repeats, ignoring case", () => {
    expect(cleanAlertNames([" a ", "", "A", "b"])).toEqual(["a", "b"]);
  });
});

describe("findRunbookForIncident", () => {
  const catalog = [
    { name: "Gateway", runbookUrl: "https://docs.example.com/gw", alerts: ["gw-5xx", "gw-latency"] },
    { name: "Gateway prod", runbookUrl: "https://docs.example.com/long", alerts: ["gw-5xx-prod-001"] },
    { name: "JVM", runbookUrl: null, alerts: ["JVM Heap"] },
  ];

  it("picks the runbook holding the longest alert name the subject contains, ignoring case", () => {
    const match = findRunbookForIncident(catalog, "Fired GW-5XX-PROD-001 on log");
    expect(match?.runbook.name).toBe("Gateway prod");
    expect(match?.alertName).toBe("gw-5xx-prod-001");
    expect(findRunbookForIncident(catalog, "gw-latency spike")?.runbook.name).toBe("Gateway");
  });

  it("finds a runbook without a link, and nothing for an unknown subject", () => {
    expect(findRunbookForIncident(catalog, "node / jvm heap")?.runbook.runbookUrl).toBeNull();
    expect(findRunbookForIncident(catalog, "unrelated")).toBeUndefined();
  });
});
