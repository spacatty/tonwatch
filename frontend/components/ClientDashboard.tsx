"use client";

import dynamic from "next/dynamic";
import { AuthGate } from "./Auth";

// The dashboard is a live, per-account tool (clocks, countdowns, theme, saved filters):
// render it in the browser only, so nothing time-dependent or personal is prerendered.
const Dashboard = dynamic(() => import("./Dashboard"), {
  ssr: false,
  loading: () => <div className="min-h-dvh bg-background" />,
});

export default function ClientDashboard() {
  return (
    <AuthGate>
      <Dashboard />
    </AuthGate>
  );
}
