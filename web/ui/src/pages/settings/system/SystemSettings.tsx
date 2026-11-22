import PageHeader from "@/components/PageHeader";
import SystemInfo from "@beacon-shared/SystemInfo";
import TasksCard from "@beacon-shared/TasksCard";
import HealthPanel from "./HealthPanel";

export default function SystemSettings() {
  return (
    <div style={{ padding: 24, maxWidth: 900 }}>
      <PageHeader title="System" description="What's running and how it's doing." />
      <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
        <SystemInfo app="Pilot" service="pilot" image="ghcr.io/beacon-stack/pilot" />
        <HealthPanel />
        <TasksCard />
      </div>
    </div>
  );
}
