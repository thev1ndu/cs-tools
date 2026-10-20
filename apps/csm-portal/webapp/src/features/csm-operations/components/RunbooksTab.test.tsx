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

import { fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import RunbooksTab from "@features/csm-operations/components/RunbooksTab";

// The import tree reads this at module load; vitest does not load public/config.js.
vi.hoisted(() => {
  window.config = { ...window.config, CSM_PORTAL_BACKEND_BASE_URL: "http://localhost:9090" };
});

const mocks = vi.hoisted(() => ({ create: vi.fn(), update: vi.fn() }));

const catalog = [
  {
    id: "1",
    name: "Gateway",
    runbookUrl: "https://docs.example.com/gw",
    alerts: ["gw-5xx", "Gateway Latency High"],
    createdOn: "",
    createdBy: "",
    updatedOn: "",
    updatedBy: "",
  },
  { id: "2", name: "JVM", runbookUrl: null, alerts: ["JVM Heap"], createdOn: "", createdBy: "", updatedOn: "", updatedBy: "" },
];

const idleMutation = (mutate = vi.fn()) => ({ mutate, isPending: false, isError: false, error: null });

vi.mock("@context/current-user/usePortalAccess", () => ({ usePortalAccess: () => ({ canWrite: true }) }));
vi.mock("@features/csm-operations/api/useRunbooks", () => ({
  useRunbooks: () => ({ data: catalog, isLoading: false, isError: false, error: null, refetch: vi.fn() }),
  useCreateRunbook: () => idleMutation(mocks.create),
  useUpdateRunbook: () => idleMutation(mocks.update),
  useDeleteRunbook: () => idleMutation(),
  useImportRunbooks: () => idleMutation(),
}));

function type(el: HTMLElement, value: string): void {
  fireEvent.change(el, { target: { value } });
}

function addTag(el: HTMLElement, value: string): void {
  type(el, value);
  fireEvent.keyDown(el, { key: "Enter" });
}

describe("RunbooksTab", () => {
  beforeEach(() => {
    mocks.create.mockReset();
    mocks.update.mockReset();
  });

  it("lists each runbook with its name, link and alert count", () => {
    render(<RunbooksTab />);
    const row = screen.getByText("Gateway").closest("tr") as HTMLElement;
    expect(within(row).getByRole("link", { name: /Open runbook/ })).toHaveAttribute("href", "https://docs.example.com/gw");
    const count = within(row).getByText("2 alerts");
    expect(count).toHaveAttribute("tabindex", "0");
    expect(count).toHaveAttribute("aria-label", "2 alerts: gw-5xx, Gateway Latency High");
    expect(within(row).queryByText("gw-5xx")).not.toBeInTheDocument();
    expect(within(screen.getByText("JVM").closest("tr") as HTMLElement).getByText("1 alert")).toBeInTheDocument();
    expect(screen.getByText("2 runbooks covering 3 alerts, 1 without a link")).toBeInTheDocument();
  });

  it("adds alerts as tags with Enter and saves them with the name", () => {
    render(<RunbooksTab />);
    fireEvent.click(screen.getByRole("button", { name: "Add runbook" }));
    const dialog = screen.getByRole("dialog");
    type(within(dialog).getByLabelText(/^Name/), "Disk");
    const alerts = within(dialog).getByLabelText(/^Alerts/);
    addTag(alerts, "Disk Space Low");
    type(alerts, "disk-full");
    expect(within(dialog).getByText("Disk Space Low")).toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Add" }));
    expect(mocks.create).toHaveBeenCalledWith(
      { name: "Disk", runbookUrl: null, alerts: ["Disk Space Low", "disk-full"] },
      expect.anything(),
    );
  });

  it("refuses an alert another runbook already has", () => {
    render(<RunbooksTab />);
    fireEvent.click(screen.getByRole("button", { name: "Add runbook" }));
    const dialog = screen.getByRole("dialog");
    type(within(dialog).getByLabelText(/^Name/), "Other");
    addTag(within(dialog).getByLabelText(/^Alerts/), "JVM HEAP");
    expect(within(dialog).getByText(/Already used by another runbook: JVM HEAP \(in JVM\)/)).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Add" })).toBeDisabled();
  });

  it("edits a runbook's alerts", () => {
    render(<RunbooksTab />);
    fireEvent.click(screen.getByRole("button", { name: "Edit Gateway" }));
    const dialog = screen.getByRole("dialog");
    addTag(within(dialog).getByLabelText(/^Alerts/), "gw-timeout");
    fireEvent.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(mocks.update).toHaveBeenCalledWith(
      {
        id: "1",
        input: { name: "Gateway", runbookUrl: "https://docs.example.com/gw", alerts: ["gw-5xx", "Gateway Latency High", "gw-timeout"] },
      },
      expect.anything(),
    );
  });

  it("shows Anyone as the default editor filter, like the other filters show their default", () => {
    render(<RunbooksTab />);
    fireEvent.click(screen.getByRole("button", { name: "Filters" }));
    expect(screen.getByRole("combobox", { name: "Last updated by" })).toHaveTextContent("Anyone");
    expect(screen.getByRole("combobox", { name: "Last updated" })).toHaveTextContent("Any time");
  });

  it("searches the runbook with an alert name", () => {
    render(<RunbooksTab />);
    const search = screen.getByLabelText("Search runbook with alert name");
    fireEvent.change(search, { target: { value: "gateway latency high" } });
    expect(screen.getByText("Matched alert: Gateway Latency High")).toBeInTheDocument();
    fireEvent.change(search, { target: { value: "unknown" } });
    expect(screen.getByText("No runbook covers this alert.")).toBeInTheDocument();
  });
});
