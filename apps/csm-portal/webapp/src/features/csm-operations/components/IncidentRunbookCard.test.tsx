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

import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import IncidentRunbookCard from "@features/csm-operations/components/IncidentRunbookCard";

const runbooksState = vi.hoisted(() => ({
  value: { data: undefined as unknown, isLoading: false, isError: false },
}));

vi.mock("@features/csm-operations/api/useRunbooks", () => ({
  useRunbooks: () => runbooksState.value,
}));

const catalog = [
  { name: "Gateway", runbookUrl: "https://docs.example.com/gw-5xx", alerts: ["gw-5xx"] },
  { name: "Example gateway", runbookUrl: "https://docs.example.com/example-gw-5xx", alerts: ["gw-latency", "example-gw-5xx"] },
  { name: "JVM", runbookUrl: null, alerts: ["JVM Heap"] },
];

describe("IncidentRunbookCard", () => {
  beforeEach(() => {
    runbooksState.value = { data: catalog, isLoading: false, isError: false };
  });

  it("links the runbook of the longest alert name the subject contains", () => {
    render(<IncidentRunbookCard subject="[Alert] Fired: EXAMPLE-GW-5xx on prod" />);
    expect(screen.getByText("Example gateway")).toBeInTheDocument();
    expect(screen.getByText("example-gw-5xx")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Open runbook/ })).toHaveAttribute(
      "href",
      "https://docs.example.com/example-gw-5xx",
    );
  });

  it("says a matched alert has no runbook yet", () => {
    render(<IncidentRunbookCard subject="Icinga Alert: node-01 / JVM Heap" />);
    expect(screen.getByText("JVM Heap")).toBeInTheDocument();
    expect(screen.getByText("No runbook")).toBeInTheDocument();
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
  });

  it("says when no alert matches", () => {
    render(<IncidentRunbookCard subject="Login page is slow" />);
    expect(screen.getByText("No runbook matches this incident.")).toBeInTheDocument();
  });

  it("says when the catalog cannot be loaded", () => {
    runbooksState.value = { data: undefined, isLoading: false, isError: true };
    render(<IncidentRunbookCard subject="gw-5xx" />);
    expect(screen.getByText("Could not load runbooks.")).toBeInTheDocument();
  });
});
