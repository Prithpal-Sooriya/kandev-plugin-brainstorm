// Entry point: registers the /brainstorm page, its sidebar nav item, and the
// task-workspace panel.
import type { PluginHostApi, PluginRegistry, PluginTaskPanelProps } from "@kandev/plugin-sdk";
import { BrainstormApp } from "./app";
import { bindHost, h, host, React } from "./runtime";

const PLUGIN_ID = "kandev-plugin-brainstorm";
const ROUTE = "/brainstorm";

function LightbulbIcon(props: { className?: string; size?: number }) {
  const size = props.size ?? 16;
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={props.className}
      aria-hidden="true"
    >
      <path d="M3 12h1m8 -9v1m8 8h1m-15.4 -6.4l.7 .7m12.1 -.7l-.7 .7" />
      <path d="M9 16a5 5 0 1 1 6 0a3.5 3.5 0 0 0 -1 3a2 2 0 0 1 -4 0a3.5 3.5 0 0 0 -1 -3" />
      <path d="M9.7 17l4.6 0" />
    </svg>
  );
}

function useActiveWorkspace(): string | null {
  const [id, setId] = React.useState<string | null>(() => host.context.getActiveWorkspaceId() ?? null);
  React.useEffect(() => host.context.subscribeActiveWorkspace((next) => setId(next ?? null)), []);
  return id;
}

// Below this page width the three columns get too tight; switch to the
// tabbed compact layout with a session dropdown.
const COMPACT_BELOW_PX = 980;

function useNarrow(threshold: number) {
  const [value, setValue] = React.useState(false);
  const [el, setEl] = React.useState<HTMLElement | null>(null);
  React.useEffect(() => {
    if (!el) return undefined;
    // Measure once now: ResizeObserver only reports on the next layout frame.
    setValue(el.getBoundingClientRect().width < threshold);
    if (typeof ResizeObserver === "undefined") return undefined;
    const observer = new ResizeObserver(([entry]) => setValue(entry.contentRect.width < threshold));
    observer.observe(el);
    return () => observer.disconnect();
  }, [el, threshold]);
  return { value, ref: setEl };
}

function BrainstormPage() {
  const workspaceId = useActiveWorkspace();
  const writerId = React.useMemo(() => host.utils.generateUUID(), []);
  const { isMobile } = host.useResponsiveBreakpoint();
  const narrow = useNarrow(COMPACT_BELOW_PX);
  const scope = React.useMemo(
    () => (workspaceId ? { scope: "workspace" as const, scopeId: workspaceId, single: false } : null),
    [workspaceId],
  );
  if (!scope) {
    return <div className="kbs-empty kbs-muted">Select a workspace to start brainstorming.</div>;
  }
  return (
    <div className="kbs-page" ref={narrow.ref}>
      <BrainstormApp
        key={scope.scopeId}
        scope={scope}
        workspaceId={workspaceId}
        compact={isMobile || narrow.value}
        writerId={writerId}
      />
    </div>
  );
}

function BrainstormTaskPanel(props: PluginTaskPanelProps) {
  const workspaceId = useActiveWorkspace();
  const scope = React.useMemo(() => ({ scope: "task" as const, scopeId: props.taskId, single: true }), [props.taskId]);
  return (
    <div className="kbs-panel">
      <BrainstormApp
        key={props.taskId}
        scope={scope}
        workspaceId={workspaceId}
        taskId={props.taskId}
        compact
        writerId={props.panelId}
      />
    </div>
  );
}

window.registerKandevPlugin(PLUGIN_ID, {
  initialize(registry: PluginRegistry, api: PluginHostApi) {
    bindHost(api);
    registry.registerRoute(ROUTE, BrainstormPage, {
      topbar: { title: "Brainstorm", subtitle: "Think out loud with an agent", icon: LightbulbIcon },
    });
    registry.registerNavItem({ id: "brainstorm", label: "Brainstorm", path: ROUTE, icon: LightbulbIcon, section: "main" });
    registry.registerTaskPanel({
      id: "brainstorm",
      title: "Brainstorm",
      icon: LightbulbIcon,
      mobileEnabled: true,
      Component: BrainstormTaskPanel,
    });
  },
});

declare global {
  interface Window {
    registerKandevPlugin(
      id: string,
      plugin: { initialize(registry: PluginRegistry, host: PluginHostApi): void; destroy?(): void },
    ): void;
  }
}
