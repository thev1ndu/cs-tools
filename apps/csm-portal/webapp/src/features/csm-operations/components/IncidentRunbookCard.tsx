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

import { Box, Card, Chip, Link, Skeleton, Typography } from "@wso2/oxygen-ui";
import { BookOpen, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useMemo } from "react";
import { useRunbooks } from "@features/csm-operations/api/useRunbooks";
import { findRunbookForIncident } from "@features/csm-operations/utils/runbooks";

interface IncidentRunbookCardProps {
  subject: string;
}

/** The incident's runbook from Operations > Runbooks: the one holding the longest alert name its subject contains. */
export default function IncidentRunbookCard({ subject }: IncidentRunbookCardProps): JSX.Element {
  const { data: runbooks, isLoading, isError } = useRunbooks();
  const match = useMemo(
    () => (runbooks ? findRunbookForIncident(runbooks, subject) : undefined),
    [runbooks, subject],
  );

  let body: JSX.Element;
  if (isLoading) {
    body = <Skeleton variant="text" width="60%" />;
  } else if (isError) {
    body = (
      <Typography variant="body2" color="text.secondary">
        Could not load runbooks.
      </Typography>
    );
  } else if (!match) {
    body = (
      <Typography variant="body2" color="text.secondary">
        No runbook matches this incident.
      </Typography>
    );
  } else {
    body = (
      <Box sx={{ display: "flex", flexDirection: "column", gap: 1 }}>
        <Typography variant="body2" sx={{ wordBreak: "break-word" }}>
          {match.runbook.name}
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ wordBreak: "break-word" }}>
          Matched alert: <Box component="code">{match.alertName}</Box>
        </Typography>
        {match.runbook.runbookUrl ? (
          <Link
            href={match.runbook.runbookUrl}
            target="_blank"
            rel="noopener noreferrer"
            sx={{ display: "inline-flex", alignItems: "center", gap: 0.5, alignSelf: "flex-start" }}
          >
            Open runbook
            <ExternalLink size={14} />
          </Link>
        ) : (
          <Chip size="small" variant="outlined" color="warning" label="No runbook" sx={{ alignSelf: "flex-start" }} />
        )}
      </Box>
    );
  }

  return (
    <Card sx={{ p: 2.5, display: "flex", flexDirection: "column", gap: 1.5 }}>
      <Box sx={{ display: "flex", alignItems: "center", gap: 1 }}>
        <BookOpen size={16} />
        <Typography variant="subtitle2">Runbook</Typography>
      </Box>
      {body}
    </Card>
  );
}
