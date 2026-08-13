import { Outlet } from "react-router-dom";
import { twc } from "react-twc";
import { Nav } from "./Nav.tsx";

const AppShell = twc.div`flex min-h-screen bg-gray-50`;

const Sidebar = twc.aside`
  hidden md:flex md:flex-col md:w-56 md:fixed md:inset-y-0
  border-r border-gray-200 bg-white
`;

const SidebarHeader = twc.div`
  flex h-16 items-center gap-2 border-b border-gray-200 px-4
`;

const SidebarLogo = twc.span`text-lg font-bold text-gray-900`;

const SidebarSubtitle = twc.span`text-xs text-gray-400 ml-0.5`;

const SidebarBody = twc.div`flex-1 overflow-y-auto px-3 py-4`;

const MainArea = twc.div`flex flex-1 flex-col md:pl-56`;

const TopBar = twc.header`
  flex h-16 items-center gap-4 border-b border-gray-200 bg-white px-6 md:hidden
`;

const TopBarTitle = twc.span`text-base font-semibold text-gray-900`;

const PageContent = twc.main`flex-1 overflow-y-auto p-6`;

export function Layout() {
  return (
    <AppShell>
      <Sidebar>
        <SidebarHeader>
          <SidebarLogo>鲁班</SidebarLogo>
          <SidebarSubtitle>Luban</SidebarSubtitle>
        </SidebarHeader>
        <SidebarBody>
          <Nav />
        </SidebarBody>
      </Sidebar>
      <MainArea>
        <TopBar>
          <TopBarTitle>鲁班 Luban</TopBarTitle>
        </TopBar>
        <PageContent>
          <Outlet />
        </PageContent>
      </MainArea>
    </AppShell>
  );
}
