import { useQuery } from "@tanstack/react-query";
import { apiFetch } from "./client";
import type { CalendarEpisode } from "@/types";

export function useCalendar(start: string, end: string) {
  return useQuery({
    queryKey: ["calendar", start, end],
    queryFn: () =>
      apiFetch<CalendarEpisode[]>(
        `/calendar?start=${encodeURIComponent(start)}&end=${encodeURIComponent(end)}`
      ),
    enabled: !!start && !!end,
  });
}
