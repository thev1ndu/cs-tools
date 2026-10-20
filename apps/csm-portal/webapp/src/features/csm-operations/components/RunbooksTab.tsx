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

import {
  Alert,
  Autocomplete,
  Box,
  Button,
  Checkbox,
  Chip,
  Collapse,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  Divider,
  FormControl,
  Grid,
  IconButton,
  InputAdornment,
  InputLabel,
  Link,
  MenuItem,
  Paper,
  Select,
  Skeleton,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TablePagination,
  TableRow,
  TextField,
  Tooltip,
  Typography,
} from "@wso2/oxygen-ui";
import { ChevronDown, ChevronUp, ExternalLink, ListFilter, Pencil, Plus, Search, Trash2, Upload, X } from "@wso2/oxygen-ui-icons-react";
import { Fragment, useMemo, useRef, useState, type ChangeEvent, type JSX } from "react";
import QueryErrorState from "@components/QueryErrorState";
import { usePortalAccess } from "@context/current-user/usePortalAccess";
import {
  useCreateRunbook,
  useDeleteRunbook,
  useImportRunbooks,
  useMergeRunbooks,
  useRunbooks,
  useUnmergeRunbookAlerts,
  useUpdateRunbook,
} from "@features/csm-operations/api/useRunbooks";
import {
  activeRunbookFilterCount,
  cleanAlertNames,
  DEFAULT_RUNBOOK_FILTERS,
  filterRunbooks,
  findRunbookForIncident,
  isValidRunbookUrl,
  parseRunbookImport,
  type Runbook,
  type RunbookFilters,
} from "@features/csm-operations/utils/runbooks";

const DEFAULT_ROWS_PER_PAGE = 20;
// A Select treats "" as no selection and leaves its label inside the box, so "Anyone" needs a real value.
const ANY_EDITOR = "__anyone__";
const ROWS_PER_PAGE_OPTIONS = [10, 20, 50];

interface RunbookDialogState {
  open: boolean;
  editing?: Runbook;
}

function errorText(err: unknown, fallback: string): string {
  return err instanceof Error && err.message.trim() ? err.message : fallback;
}

/**
 * Operations -> Runbooks: named runbook links, each with the alert names it covers.
 * An incident whose subject contains one of those alert names shows the runbook in
 * its Related tab.
 */
export default function RunbooksTab(): JSX.Element {
  const { canWrite } = usePortalAccess();
  const { data: runbooks = [], isLoading, isError, error, refetch } = useRunbooks();
  const deleteRunbook = useDeleteRunbook();
  const importRunbooks = useImportRunbooks();
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [search, setSearch] = useState("");
  const [filters, setFilters] = useState<RunbookFilters>(DEFAULT_RUNBOOK_FILTERS);
  const [isFiltersOpen, setIsFiltersOpen] = useState(false);
  const [page, setPage] = useState(0);
  const [rowsPerPage, setRowsPerPage] = useState(DEFAULT_ROWS_PER_PAGE);
  const [dialog, setDialog] = useState<RunbookDialogState>({ open: false });
  const [pendingDelete, setPendingDelete] = useState<Runbook | null>(null);
  const [notice, setNotice] = useState<{ severity: "success" | "error"; text: string } | null>(null);
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
  const [selectedIds, setSelectedIds] = useState<ReadonlySet<string>>(() => new Set());
  const [isMergeOpen, setIsMergeOpen] = useState(false);
  const unmerge = useUnmergeRunbookAlerts();

  const linkedCount = runbooks.filter((r) => r.runbookUrl).length;
  const alertCount = runbooks.reduce((n, r) => n + r.alerts.length, 0);

  const filtered = useMemo(() => filterRunbooks(runbooks, search, filters), [runbooks, search, filters]);
  const activeFilters = activeRunbookFilterCount(filters);
  const editors = useMemo(
    () => [...new Set(runbooks.map((r) => r.updatedBy).filter(Boolean))].sort((a, b) => a.localeCompare(b)),
    [runbooks],
  );

  const setFilter = <K extends keyof RunbookFilters>(key: K, value: RunbookFilters[K]): void => {
    setFilters((f) => ({ ...f, [key]: value }));
    setPage(0);
  };

  const pageRows = filtered.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);
  // Ids of runbooks that no longer exist (merged away, deleted) drop out on their own.
  const selected = runbooks.filter((r) => selectedIds.has(r.id));
  const columnCount = canWrite ? 5 : 3;

  const toggle = (set: ReadonlySet<string>, id: string): Set<string> => {
    const next = new Set(set);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    return next;
  };

  const handleUnmerge = (rb: Runbook, alert: string): void => {
    unmerge.mutate(
      { id: rb.id, alerts: [alert] },
      {
        onSuccess: () => setNotice({ severity: "success", text: `Moved ${alert} out of ${rb.name} into a runbook of its own.` }),
        onError: (err) => setNotice({ severity: "error", text: errorText(err, "Failed to unmerge the alert.") }),
      },
    );
  };

  const handleDelete = (): void => {
    if (!pendingDelete) return;
    deleteRunbook.mutate(pendingDelete.id, {
      onSuccess: () => setPendingDelete(null),
    });
  };

  const handleImportFile = async (e: ChangeEvent<HTMLInputElement>): Promise<void> => {
    const file = e.target.files?.[0];
    e.target.value = "";
    if (!file) return;
    try {
      const entries = parseRunbookImport(await file.text());
      if (entries.length === 0) {
        setNotice({ severity: "error", text: "The file has no runbooks." });
        return;
      }
      importRunbooks.mutate(entries, {
        onSuccess: (res) =>
          setNotice({ severity: "success", text: `Imported ${res.created} new and updated ${res.updated} existing runbooks.` }),
        onError: (err) => setNotice({ severity: "error", text: errorText(err, "Failed to import runbooks.") }),
      });
    } catch (err) {
      setNotice({ severity: "error", text: errorText(err, "The file could not be read.") });
    }
  };

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
      <Box sx={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
        <Typography variant="body2" color="text.secondary">
          {isLoading
            ? "Loading runbooks…"
            : `${runbooks.length} runbooks covering ${alertCount} alerts, ${runbooks.length - linkedCount} without a link`}
        </Typography>
        {canWrite && (
          <Box sx={{ display: "flex", gap: 1 }}>
            <input
              ref={fileInputRef}
              type="file"
              accept="application/json,.json"
              hidden
              onChange={(e) => void handleImportFile(e)}
              data-testid="runbook-import-input"
            />
            <Button
              variant="outlined"
              size="small"
              startIcon={<Upload size={16} />}
              disabled={importRunbooks.isPending}
              onClick={() => fileInputRef.current?.click()}
            >
              {importRunbooks.isPending ? "Importing…" : "Import JSON"}
            </Button>
            <Button
              variant="contained"
              color="primary"
              size="small"
              startIcon={<Plus size={16} />}
              onClick={() => setDialog({ open: true })}
            >
              Add runbook
            </Button>
          </Box>
        )}
      </Box>

      {notice && (
        <Alert severity={notice.severity} onClose={() => setNotice(null)}>
          {notice.text}
        </Alert>
      )}

      <Paper sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 1.5 }}>
        <Box sx={{ display: "flex", alignItems: "center", gap: 1.5, flexWrap: "wrap" }}>
          <Box sx={{ flex: 1, minWidth: 240 }}>
            <TextField
              size="small"
              placeholder="Search by name, alert name or runbook link…"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(0);
              }}
              fullWidth
              slotProps={{
                htmlInput: { "aria-label": "Search runbooks by name, alert name or link" },
                input: {
                  startAdornment: (
                    <InputAdornment position="start">
                      <Search size={16} />
                    </InputAdornment>
                  ),
                  endAdornment: search ? (
                    <InputAdornment position="end">
                      <IconButton size="small" edge="end" onClick={() => setSearch("")} aria-label="Clear search">
                        <X size={16} />
                      </IconButton>
                    </InputAdornment>
                  ) : undefined,
                },
              }}
            />
          </Box>

          <Button
            variant="outlined"
            size="small"
            color="primary"
            onClick={() => setIsFiltersOpen((open) => !open)}
            startIcon={<ListFilter size={16} />}
            endIcon={isFiltersOpen ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
          >
            {activeFilters > 0 ? `Filters (${activeFilters})` : "Filters"}
          </Button>
          {activeFilters > 0 && (
            <Button
              variant="text"
              size="small"
              color="primary"
              onClick={() => {
                setFilters(DEFAULT_RUNBOOK_FILTERS);
                setPage(0);
              }}
              startIcon={<X size={16} />}
            >
              Clear filters
            </Button>
          )}
        </Box>

        {isFiltersOpen && (
          <>
            <Divider />
            <Grid container spacing={2} alignItems="center">
              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                <FormControl fullWidth size="small">
                  <InputLabel id="runbook-filter-coverage-label">Runbook link</InputLabel>
                  <Select
                    labelId="runbook-filter-coverage-label"
                    id="runbook-filter-coverage"
                    label="Runbook link"
                    value={filters.coverage}
                    onChange={(e) => setFilter("coverage", e.target.value as RunbookFilters["coverage"])}
                  >
                    <MenuItem value="all">All</MenuItem>
                    <MenuItem value="linked">Has a link</MenuItem>
                    <MenuItem value="missing">No link yet</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                <FormControl fullWidth size="small">
                  <InputLabel id="runbook-filter-alerts-label">Alerts</InputLabel>
                  <Select
                    labelId="runbook-filter-alerts-label"
                    id="runbook-filter-alerts"
                    label="Alerts"
                    value={filters.alertCount}
                    onChange={(e) => setFilter("alertCount", e.target.value as RunbookFilters["alertCount"])}
                  >
                    <MenuItem value="all">All</MenuItem>
                    <MenuItem value="single">One alert</MenuItem>
                    <MenuItem value="multiple">Several alerts</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                <FormControl fullWidth size="small">
                  <InputLabel id="runbook-filter-updated-by-label">Last updated by</InputLabel>
                  <Select
                    labelId="runbook-filter-updated-by-label"
                    id="runbook-filter-updated-by"
                    label="Last updated by"
                    value={filters.updatedBy || ANY_EDITOR}
                    onChange={(e) => setFilter("updatedBy", e.target.value === ANY_EDITOR ? "" : e.target.value)}
                  >
                    <MenuItem value={ANY_EDITOR}>Anyone</MenuItem>
                    {editors.map((who) => (
                      <MenuItem key={who} value={who}>
                        {who}
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              </Grid>
              <Grid size={{ xs: 12, sm: 6, md: 3 }}>
                <FormControl fullWidth size="small">
                  <InputLabel id="runbook-filter-updated-within-label">Last updated</InputLabel>
                  <Select
                    labelId="runbook-filter-updated-within-label"
                    id="runbook-filter-updated-within"
                    label="Last updated"
                    value={filters.updatedWithin}
                    onChange={(e) => setFilter("updatedWithin", e.target.value as RunbookFilters["updatedWithin"])}
                  >
                    <MenuItem value="all">Any time</MenuItem>
                    <MenuItem value="7">Past 7 days</MenuItem>
                    <MenuItem value="30">Past 30 days</MenuItem>
                    <MenuItem value="90">Past 90 days</MenuItem>
                  </Select>
                </FormControl>
              </Grid>
            </Grid>
          </>
        )}
      </Paper>

      {canWrite && selected.length > 0 && (
        <Box sx={{ display: "flex", alignItems: "center", gap: 1, flexWrap: "wrap" }}>
          <Typography variant="body2" color="text.secondary">
            {selected.length} selected
          </Typography>
          <Tooltip title={selected.length < 2 ? "Select at least two runbooks to merge." : ""}>
            <span>
              <Button variant="contained" size="small" disabled={selected.length < 2} onClick={() => setIsMergeOpen(true)}>
                Merge
              </Button>
            </span>
          </Tooltip>
          <Button variant="text" size="small" onClick={() => setSelectedIds(new Set())}>
            Clear selection
          </Button>
        </Box>
      )}

      <Box sx={{ border: 1, borderColor: "divider", borderRadius: 1, overflow: "hidden" }}>
        <TableContainer>
          <Table size="small" sx={{ "& .MuiTableCell-root": { borderColor: "divider" } }}>
            <TableHead>
              <TableRow sx={{ bgcolor: "action.hover" }}>
                {canWrite && <TableCell padding="checkbox" />}
                <TableCell padding="checkbox" />
                <TableCell>Name</TableCell>
                <TableCell sx={{ width: 180 }}>Runbook</TableCell>
                {canWrite && <TableCell align="right" sx={{ width: 96 }}>Actions</TableCell>}
              </TableRow>
            </TableHead>
            <TableBody>
              {isLoading ? (
                Array.from({ length: rowsPerPage }).map((_, i) => (
                  <TableRow key={i}>
                    {canWrite && <TableCell padding="checkbox" />}
                    <TableCell padding="checkbox" />
                    <TableCell><Skeleton variant="rounded" width="70%" height={18} /></TableCell>
                    <TableCell><Skeleton variant="rounded" width={110} height={22} /></TableCell>
                    {canWrite && <TableCell><Skeleton variant="rounded" width={56} height={18} /></TableCell>}
                  </TableRow>
                ))
              ) : isError ? (
                <TableRow>
                  <TableCell colSpan={columnCount} align="center">
                    <QueryErrorState message={errorText(error, "Failed to load runbooks.")} error={error} onRetry={() => void refetch()} />
                  </TableCell>
                </TableRow>
              ) : pageRows.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={columnCount} align="center" sx={{ py: 4 }}>
                    <Typography variant="body2" color="text.secondary">
                      No runbooks found.
                    </Typography>
                  </TableCell>
                </TableRow>
              ) : (
                pageRows.map((rb) => {
                  const open = expanded.has(rb.id);
                  return (
                    <Fragment key={rb.id}>
                      <TableRow hover sx={open ? { "& > td": { borderBottom: 0 } } : {}}>
                        {canWrite && (
                          <TableCell padding="checkbox">
                            <Checkbox
                              size="small"
                              checked={selectedIds.has(rb.id)}
                              onChange={() => setSelectedIds((set) => toggle(set, rb.id))}
                              slotProps={{ input: { "aria-label": `Select ${rb.name}` } }}
                            />
                          </TableCell>
                        )}
                        <TableCell padding="checkbox">
                          <IconButton
                            size="small"
                            aria-label={`${open ? "Hide" : "Show"} the alerts of ${rb.name}`}
                            aria-expanded={open}
                            onClick={() => setExpanded((set) => toggle(set, rb.id))}
                          >
                            {open ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                          </IconButton>
                        </TableCell>
                        <TableCell sx={{ wordBreak: "break-word" }}>
                          <Typography variant="body2">{rb.name}</Typography>
                          <Typography variant="caption" color="text.secondary">
                            {rb.alerts.length} {rb.alerts.length === 1 ? "alert" : "alerts"}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          {rb.runbookUrl ? (
                            <Link
                              href={rb.runbookUrl}
                              target="_blank"
                              rel="noopener noreferrer"
                              sx={{ display: "inline-flex", alignItems: "center", gap: 0.5 }}
                            >
                              Open runbook
                              <ExternalLink size={14} />
                            </Link>
                          ) : (
                            <Chip size="small" variant="outlined" color="warning" label="No runbook" />
                          )}
                        </TableCell>
                        {canWrite && (
                          <TableCell align="right">
                            <Tooltip title="Edit">
                              <IconButton size="small" aria-label={`Edit ${rb.name}`} onClick={() => setDialog({ open: true, editing: rb })}>
                                <Pencil size={16} />
                              </IconButton>
                            </Tooltip>
                            <Tooltip title="Delete">
                              <IconButton size="small" aria-label={`Delete ${rb.name}`} onClick={() => setPendingDelete(rb)}>
                                <Trash2 size={16} />
                              </IconButton>
                            </Tooltip>
                          </TableCell>
                        )}
                      </TableRow>
                      <TableRow>
                        <TableCell colSpan={columnCount} sx={{ py: 0, ...(open ? {} : { borderBottom: 0 }) }}>
                          <Collapse in={open} unmountOnExit>
                            <RunbookAlertList
                              runbook={rb}
                              canWrite={canWrite}
                              busy={unmerge.isPending}
                              onUnmerge={(alert) => handleUnmerge(rb, alert)}
                            />
                          </Collapse>
                        </TableCell>
                      </TableRow>
                    </Fragment>
                  );
                })
              )}
            </TableBody>
          </Table>
        </TableContainer>
        <TablePagination
          component="div"
          count={filtered.length}
          page={page}
          onPageChange={(_: unknown, next: number) => setPage(next)}
          rowsPerPage={rowsPerPage}
          onRowsPerPageChange={(e: ChangeEvent<HTMLInputElement>) => {
            setRowsPerPage(parseInt(e.target.value, 10));
            setPage(0);
          }}
          rowsPerPageOptions={ROWS_PER_PAGE_OPTIONS}
          labelDisplayedRows={({ from, to, count }) => `Showing ${from}–${to} of ${count}`}
          showFirstButton
          showLastButton
        />
      </Box>

      <RunbookAlertSearch runbooks={runbooks} />

      {isMergeOpen && selected.length >= 2 && (
        <MergeRunbooksDialog
          selected={selected}
          existing={runbooks}
          onClose={() => setIsMergeOpen(false)}
          onMerged={(merged, count) => {
            setIsMergeOpen(false);
            setSelectedIds(new Set());
            setExpanded((set) => new Set(set).add(merged.id));
            setNotice({ severity: "success", text: `Merged ${count} runbooks into ${merged.name}.` });
          }}
        />
      )}

      {dialog.open && (
        <RunbookDialog
          editing={dialog.editing}
          existing={runbooks}
          onClose={() => setDialog({ open: false })}
        />
      )}

      <Dialog open={pendingDelete !== null} onClose={() => setPendingDelete(null)} maxWidth="xs" fullWidth>
        <DialogTitle>Delete runbook?</DialogTitle>
        <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2 }}>
          <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
            Incidents matching the {pendingDelete?.alerts.length === 1 ? "alert" : `${pendingDelete?.alerts.length ?? 0} alerts`} of{" "}
            <strong>{pendingDelete?.name}</strong> will no longer show this runbook.
          </Typography>
          {deleteRunbook.isError && (
            <Alert severity="error">{errorText(deleteRunbook.error, "Failed to delete the runbook.")}</Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPendingDelete(null)}>Go back</Button>
          <Button color="error" variant="contained" onClick={handleDelete} disabled={deleteRunbook.isPending}>
            Delete
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
}

interface RunbookDialogProps {
  editing?: Runbook;
  existing: Runbook[];
  onClose: () => void;
}

function RunbookAlertList({
  runbook,
  canWrite,
  busy,
  onUnmerge,
}: {
  runbook: Runbook;
  canWrite: boolean;
  busy: boolean;
  onUnmerge: (alert: string) => void;
}): JSX.Element {
  const onlyOne = runbook.alerts.length === 1;
  return (
    <Box
      component="ul"
      aria-label={`Alerts of ${runbook.name}`}
      sx={{ listStyle: "none", m: 0, py: 1, pl: canWrite ? 13 : 7, pr: 2, display: "flex", flexDirection: "column", gap: 0.5 }}
    >
      {runbook.alerts.map((alert) => (
        <Box component="li" key={alert} sx={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 2 }}>
          <Typography variant="body2" sx={{ wordBreak: "break-word", fontFamily: "monospace" }}>
            {alert}
          </Typography>
          {canWrite && (
            <Tooltip title={onlyOne ? "A runbook keeps at least one alert." : "Move this alert into a runbook of its own."}>
              <span>
                <Button size="small" variant="text" disabled={onlyOne || busy} onClick={() => onUnmerge(alert)} aria-label={`Unmerge ${alert}`}>
                  Unmerge
                </Button>
              </span>
            </Tooltip>
          )}
        </Box>
      ))}
    </Box>
  );
}

function MergeRunbooksDialog({
  selected,
  existing,
  onClose,
  onMerged,
}: {
  selected: Runbook[];
  existing: Runbook[];
  onClose: () => void;
  onMerged: (merged: Runbook, count: number) => void;
}): JSX.Element {
  const merge = useMergeRunbooks();
  const [target] = selected;
  const [name, setName] = useState(target.name);
  const [runbookUrl, setRunbookUrl] = useState(selected.find((r) => r.runbookUrl)?.runbookUrl ?? "");
  const trimmedName = name.trim();
  const selectedIds = new Set(selected.map((r) => r.id));
  const duplicateName = existing.some((r) => !selectedIds.has(r.id) && r.name.trim().toLowerCase() === trimmedName.toLowerCase());
  const urlInvalid = !isValidRunbookUrl(runbookUrl);
  const alertTotal = selected.reduce((n, r) => n + r.alerts.length, 0);
  const canMerge = trimmedName.length > 0 && !duplicateName && !urlInvalid && !merge.isPending;

  const handleMerge = (): void => {
    merge.mutate(
      { runbookIds: selected.map((r) => r.id), name: trimmedName, runbookUrl: runbookUrl.trim() || null },
      { onSuccess: (merged) => onMerged(merged, selected.length) },
    );
  };

  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Merge {selected.length} runbooks</DialogTitle>
      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: "8px !important" }}>
        <Typography variant="body2" color="text.secondary">
          The {alertTotal} alerts of these runbooks move into one runbook, and the others are removed.
        </Typography>
        <Box component="ul" sx={{ m: 0, pl: 2.5 }}>
          {selected.map((r) => (
            <Typography component="li" variant="body2" key={r.id} sx={{ wordBreak: "break-word" }}>
              {r.name} ({r.alerts.length} {r.alerts.length === 1 ? "alert" : "alerts"})
            </Typography>
          ))}
        </Box>
        <TextField
          label="Name"
          required
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={duplicateName}
          helperText={duplicateName ? "Another runbook already has this name." : undefined}
        />
        <TextField
          label="Runbook link"
          placeholder="https://docs.google.com/document/d/…"
          value={runbookUrl}
          onChange={(e) => setRunbookUrl(e.target.value)}
          error={urlInvalid}
          helperText={urlInvalid ? "Enter a full http(s) link." : "Leave empty to record that no runbook exists yet."}
        />
        {merge.error && <Alert severity="error">{errorText(merge.error, "Failed to merge the runbooks.")}</Alert>}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" disabled={!canMerge} onClick={handleMerge}>
          Merge
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function RunbookDialog({ editing, existing, onClose }: RunbookDialogProps): JSX.Element {
  const createRunbook = useCreateRunbook();
  const updateRunbook = useUpdateRunbook();
  const [name, setName] = useState(editing?.name ?? "");
  const [runbookUrl, setRunbookUrl] = useState(editing?.runbookUrl ?? "");
  const [alerts, setAlerts] = useState<string[]>(editing?.alerts ?? []);
  const [pendingAlert, setPendingAlert] = useState("");

  const saving = createRunbook.isPending || updateRunbook.isPending;
  const saveError = createRunbook.error ?? updateRunbook.error;
  const trimmedName = name.trim();
  const trimmedUrl = runbookUrl.trim();
  // A typed alert not yet confirmed with Enter is still saved.
  const alertsToSave = cleanAlertNames([...alerts, pendingAlert]);
  const others = existing.filter((r) => r.id !== editing?.id);
  const duplicateName = others.some((r) => r.name.trim().toLowerCase() === trimmedName.toLowerCase());
  const takenAlerts = alertsToSave.flatMap((a) => {
    const owner = others.find((r) => r.alerts.some((b) => b.trim().toLowerCase() === a.toLowerCase()));
    return owner ? [`${a} (in ${owner.name})`] : [];
  });
  const urlInvalid = !isValidRunbookUrl(runbookUrl);
  const canSave =
    trimmedName.length > 0 && !duplicateName && alertsToSave.length > 0 && takenAlerts.length === 0 && !urlInvalid && !saving;

  const handleSave = (): void => {
    const input = { name: trimmedName, runbookUrl: trimmedUrl || null, alerts: alertsToSave };
    if (editing) {
      updateRunbook.mutate({ id: editing.id, input }, { onSuccess: onClose });
    } else {
      createRunbook.mutate(input, { onSuccess: onClose });
    }
  };

  return (
    <Dialog open onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{editing ? "Edit runbook" : "Add runbook"}</DialogTitle>
      <DialogContent sx={{ display: "flex", flexDirection: "column", gap: 2, pt: "8px !important" }}>
        <TextField
          label="Name"
          required
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          error={duplicateName}
          helperText={duplicateName ? "Another runbook already has this name." : undefined}
        />
        <TextField
          label="Runbook link"
          placeholder="https://docs.google.com/document/d/…"
          value={runbookUrl}
          onChange={(e) => setRunbookUrl(e.target.value)}
          error={urlInvalid}
          helperText={urlInvalid ? "Enter a full http(s) link." : "Leave empty to record that no runbook exists yet."}
        />
        <Autocomplete
          multiple
          freeSolo
          options={[]}
          value={alerts}
          inputValue={pendingAlert}
          onInputChange={(_e, value, reason) => {
            if (reason !== "reset") setPendingAlert(value);
          }}
          onChange={(_e, value) => {
            setAlerts(cleanAlertNames(value as string[]));
            setPendingAlert("");
          }}
          renderTags={(value, getTagProps) =>
            value.map((option, index) => {
              const { key, ...tagProps } = getTagProps({ index });
              return <Chip key={key} size="small" label={option} {...tagProps} />;
            })
          }
          renderInput={(params) => (
            <TextField
              {...params}
              label="Alerts"
              required
              placeholder={alerts.length === 0 ? "Type an alert name and press Enter" : undefined}
              error={takenAlerts.length > 0}
              helperText={
                takenAlerts.length > 0
                  ? `Already used by another runbook: ${takenAlerts.join(", ")}`
                  : "Press Enter after each alert name. An incident whose subject contains one gets this runbook (case-insensitive)."
              }
            />
          )}
        />
        {saveError && <Alert severity="error">{errorText(saveError, "Failed to save the runbook.")}</Alert>}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button variant="contained" disabled={!canSave} onClick={handleSave}>
          {editing ? "Save" : "Add"}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

function RunbookAlertSearch({ runbooks }: { runbooks: Runbook[] }): JSX.Element {
  const [query, setQuery] = useState("");
  const match = query.trim() ? findRunbookForIncident(runbooks, query) : undefined;

  return (
    <Paper sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Box>
        <Typography variant="subtitle2">Search runbook with alert name</Typography>
        <Typography variant="body2" color="text.secondary">
          Enter an alert name, or paste an incident subject, to find the runbook that covers it.
        </Typography>
      </Box>
      <TextField
        size="small"
        placeholder="example-gw-CriticalHTTP5xxCountLimitExceeded-prod-001"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        slotProps={{ htmlInput: { "aria-label": "Search runbook with alert name" } }}
      />
      {query.trim() && (
        <Alert severity={match?.runbook.runbookUrl ? "success" : "warning"}>
          {match ? (
            <>
              <Typography variant="body2" sx={{ fontWeight: 600, wordBreak: "break-word" }}>
                {match.runbook.name}
              </Typography>
              <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
                Matched alert: {match.alertName}
              </Typography>
            </>
          ) : (
            <Typography variant="body2">No runbook covers this alert.</Typography>
          )}
          {match?.runbook.runbookUrl ? (
            <Link
              href={match.runbook.runbookUrl}
              target="_blank"
              rel="noopener noreferrer"
              variant="body2"
              sx={{ mt: 0.5, display: "inline-flex", alignItems: "center", gap: 0.5, fontWeight: 600, wordBreak: "break-all" }}
            >
              {match.runbook.runbookUrl}
              <ExternalLink size={14} />
            </Link>
          ) : match ? (
            <Typography variant="body2" sx={{ mt: 0.5, fontWeight: 600 }}>
              No runbook written for it yet.
            </Typography>
          ) : null}
        </Alert>
      )}
    </Paper>
  );
}
