import { BrowserRouter, Routes, Route } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { Toaster } from "sonner";
import "./index.css";

import { ErrorBoundary } from "@/components/ErrorBoundary";
import { Shell } from "@/layouts/Shell";
import Dashboard from "@/pages/dashboard/Dashboard";
import SeriesDetail from "@/pages/series/SeriesDetail";
import LibraryList from "@/pages/settings/libraries/LibraryList";
import QualityProfileList from "@/pages/settings/quality-profiles/QualityProfileList";
import IndexerList from "@/pages/settings/indexers/IndexerList";
import SystemSettings from "@/pages/settings/system/SystemSettings";
import AppSettings from "@/pages/settings/app/AppSettings";
import DownloadClientList from "@/pages/settings/download-clients/DownloadClientList";
import NotificationList from "@/pages/settings/notifications/NotificationList";
import MediaServerList from "@/pages/settings/media-servers/MediaServerList";
import BlocklistPage from "@/pages/settings/blocklist/BlocklistPage";
import MediaManagementPage from "@/pages/settings/media-management/MediaManagementPage";
import QualityDefinitionsPage from "@/pages/settings/quality-definitions/QualityDefinitionsPage";
import CustomFormatsPage from "@/pages/settings/custom-formats/CustomFormatsPage";
import ImportListList from "@/pages/settings/import-lists/ImportListList";
import ImportExclusions from "@/pages/settings/import-lists/ImportExclusions";
import ImportPage from "@/pages/settings/import/ImportPage";
import PlaceholderPage from "@/pages/PlaceholderPage";
import Queue from "@/pages/queue/Queue";
import CalendarPage from "@/pages/calendar/CalendarPage";
import WantedPage from "@/pages/wanted/WantedPage";
import HistoryPage from "@/pages/history/HistoryPage";
import ActivityPage from "@/pages/activity/ActivityPage";
import StatsPage from "@/pages/stats/StatsPage";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <ErrorBoundary>
        <Routes>
          <Route element={<Shell />}>
            {/* Main */}
            <Route index element={<Dashboard />} />
            <Route path="series/:id" element={<SeriesDetail />} />

            {/* Nav */}
            <Route path="activity" element={<ActivityPage />} />
            <Route path="calendar" element={<CalendarPage />} />
            <Route path="wanted" element={<WantedPage />} />
            <Route path="stats" element={<StatsPage />} />
            <Route path="queue" element={<Queue />} />
            <Route path="history" element={<HistoryPage />} />

            {/* Settings — flat, no nested layout (matches Screenarr) */}
            <Route path="settings/libraries" element={<LibraryList />} />
            <Route path="settings/media-management" element={<MediaManagementPage />} />
            <Route path="settings/quality-profiles" element={<QualityProfileList />} />
            <Route path="settings/quality-definitions" element={<QualityDefinitionsPage />} />
            <Route path="settings/custom-formats" element={<CustomFormatsPage />} />
            <Route path="settings/indexers" element={<IndexerList />} />
            <Route path="settings/download-clients" element={<DownloadClientList />} />
            <Route path="settings/notifications" element={<NotificationList />} />
            <Route path="settings/media-servers" element={<MediaServerList />} />
            <Route path="settings/import-lists" element={<ImportListList />} />
            <Route path="settings/import-exclusions" element={<ImportExclusions />} />
            <Route path="settings/blocklist" element={<BlocklistPage />} />
            <Route path="settings/import" element={<ImportPage />} />
            <Route path="settings/system" element={<SystemSettings />} />
            <Route path="settings/app" element={<AppSettings />} />
          </Route>
        </Routes>
        </ErrorBoundary>
      </BrowserRouter>
      <Toaster
        theme="dark"
        position="bottom-right"
        toastOptions={{
          style: {
            background: "var(--color-bg-elevated)",
            border: "1px solid var(--color-border-default)",
            color: "var(--color-text-primary)",
          },
        }}
      />
    </QueryClientProvider>
  );
}
