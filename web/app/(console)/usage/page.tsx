"use client";

import { PageHeader } from "@/components/common";
import { UsageExplorer } from "@/components/usage-explorer";

export default function UsagePage() {
  return (
    <UsageExplorer
      header={({ stats, actions }) => (
        <PageHeader
          title="App usage"
          description="How much each installed app is actively used, for license and subscription audits. Search an app to see every device it's installed on, used or not."
          stats={stats}
          actions={actions}
        />
      )}
    />
  );
}
