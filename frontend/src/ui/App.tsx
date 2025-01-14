import {
  ChatBubbleIcon,
  CodeIcon,
  DashboardIcon,
  FrameIcon,
  MixerHorizontalIcon,
  MixIcon,
  StarIcon,
  TableIcon,
  TimerIcon,
} from '@radix-ui/react-icons';
import { EventsOff, EventsOn } from '@wailsapp/runtime/runtime';
import { useTranslation } from 'react-i18next';
import React, { useEffect, useState } from 'react';
import { Route, Routes, useLocation, useNavigate } from 'react-router-dom';

import {
  GetKilovoltBind,
  GetLastLogs,
  IsServerReady,
} from '@wailsapp/go/main/App';
import { main } from '@wailsapp/go/models';

import { useAppDispatch, useAppSelector } from '~/store';
import { createWSClient, useAuthBypass } from '~/store/api/reducer';
import { ConnectionStatus } from '~/store/api/types';
import loggingReducer from '~/store/logging/reducer';
// @ts-expect-error Asset import
import spinner from '~/assets/icon-loading.svg';

import LogViewer from './components/LogViewer';
import Sidebar, { RouteSection } from './components/Sidebar';
import Scrollbar from './components/utils/Scrollbar';
import AuthDialog from './pages/AuthDialog';
import TwitchBotCommandsPage from './pages/BotCommands';
import TwitchBotTimersPage from './pages/BotTimers';
import ChatAlertsPage from './pages/ChatAlerts';
import Dashboard from './pages/Dashboard';
import DebugPage from './pages/Debug';
import LoyaltyConfigPage from './pages/LoyaltyConfig';
import LoyaltyQueuePage from './pages/LoyaltyQueue';
import LoyaltyRewardsPage from './pages/LoyaltyRewards';
import OnboardingPage from './pages/Onboarding';
import ServerSettingsPage from './pages/ServerSettings';
import StrimertulPage from './pages/Strimertul';
import TwitchSettingsPage from './pages/TwitchSettings';
import UISettingsPage from './pages/UISettingsPage';
import { styled, TextBlock } from './theme';

const LoadingDiv = styled('div', {
  display: 'flex',
  flexDirection: 'column',
  justifyContent: 'center',
  alignItems: 'center',
  minHeight: '100vh',
});

const Spinner = styled('img', {
  maxWidth: '100px',
});

interface LoadingProps {
  message: string;
}

function Loading({ message }: React.PropsWithChildren<LoadingProps>) {
  return (
    <LoadingDiv>
      <Spinner src={spinner as string} alt="Loading..." />
      <TextBlock>{message}</TextBlock>
    </LoadingDiv>
  );
}
const sections: RouteSection[] = [
  {
    title: 'menu.sections.monitor',
    links: [
      {
        title: 'menu.pages.monitor.dashboard',
        url: '/',
        icon: <DashboardIcon />,
      },
    ],
  },
  {
    title: 'menu.sections.strimertul',
    links: [
      {
        title: 'menu.pages.strimertul.settings',
        url: '/http',
        icon: <CodeIcon />,
      },
      {
        title: 'menu.pages.strimertul.ui-config',
        url: '/ui-config',
        icon: <MixIcon />,
      },
    ],
  },
  {
    title: 'menu.sections.twitch',
    links: [
      {
        title: 'menu.pages.twitch.configuration',
        url: '/twitch/settings',
        icon: <MixerHorizontalIcon />,
      },
      {
        title: 'menu.pages.twitch.bot-commands',
        url: '/twitch/bot/commands',
        icon: <ChatBubbleIcon />,
      },
      {
        title: 'menu.pages.twitch.bot-timers',
        url: '/twitch/bot/timers',
        icon: <TimerIcon />,
      },
      {
        title: 'menu.pages.twitch.bot-alerts',
        url: '/twitch/bot/alerts',
        icon: <FrameIcon />,
      },
    ],
  },
  {
    title: 'menu.sections.loyalty',
    links: [
      {
        title: 'menu.pages.loyalty.configuration',
        url: '/loyalty/settings',
        icon: <MixerHorizontalIcon />,
      },
      {
        title: 'menu.pages.loyalty.points',
        url: '/loyalty/users',
        icon: <TableIcon />,
      },
      {
        title: 'menu.pages.loyalty.rewards',
        url: '/loyalty/rewards',
        icon: <StarIcon />,
      },
    ],
  },
];

const Container = styled('div', {
  position: 'relative',
  display: 'flex',
  flexDirection: 'row',
  overflow: 'hidden',
  height: '100vh',
});

const PageContent = styled('main', {
  flex: 1,
  overflow: 'auto',
});

const PageWrapper = styled('div', {
  display: 'flex',
  flexDirection: 'row',
  flex: 1,
  overflow: 'hidden',
});

export default function App(): JSX.Element {
  const [ready, setReady] = useState(false);
  const client = useAppSelector((state) => state.api.client);
  const uiConfig = useAppSelector((state) => state.api.uiConfig);
  const connected = useAppSelector((state) => state.api.connectionStatus);
  const dispatch = useAppDispatch();
  const location = useLocation();
  const navigate = useNavigate();
  const [t, i18n] = useTranslation();

  const connectToKV = async () => {
    const address = await GetKilovoltBind();
    await dispatch(
      createWSClient({
        address: `ws://${address}/ws`,
      }),
    );
  };

  // Get application logs
  useEffect(() => {
    void GetLastLogs().then((logs) => {
      dispatch(loggingReducer.actions.loadedLogData(logs));
    });
    EventsOn('log-event', (event: main.LogEntry) => {
      dispatch(loggingReducer.actions.receivedEvent(event));
    });
    return () => {
      EventsOff('log-event');
    };
  }, []);

  useEffect(() => {
    void IsServerReady().then(setReady);
    EventsOn('ready', (newValue: boolean) => {
      setReady(newValue);
    });
    return () => {
      EventsOff('ready');
    };
  }, []);

  useEffect(() => {
    if (!ready) {
      return;
    }
    if (!client) {
      void connectToKV();
    }
    if (connected === ConnectionStatus.AuthenticationNeeded) {
      void dispatch(useAuthBypass());
    }
  }, [ready, connected]);

  // Sync UI changes on key change
  useEffect(() => {
    if (uiConfig?.language) {
      void i18n.changeLanguage(uiConfig?.language ?? 'en');
    }
    if (!uiConfig?.onboardingDone) {
      navigate('/setup');
    }
  }, [ready, uiConfig]);

  if (connected === ConnectionStatus.NotConnected) {
    return <Loading message={t('special.loading')} />;
  }

  if (connected === ConnectionStatus.AuthenticationNeeded) {
    return <AuthDialog />;
  }

  const showSidebar = location.pathname !== '/setup';

  return (
    <Container>
      <LogViewer />
      {showSidebar ? <Sidebar sections={sections} /> : null}
      <Scrollbar
        vertical={true}
        root={{ flex: 1 }}
        viewport={{ height: '100vh', flex: '1' }}
      >
        <PageContent>
          <PageWrapper role="main">
            <Routes>
              <Route path="/" element={<Dashboard />} />
              <Route path="/setup" element={<OnboardingPage />} />
              <Route path="/about" element={<StrimertulPage />} />
              <Route path="/debug" element={<DebugPage />} />
              <Route path="/http" element={<ServerSettingsPage />} />
              <Route path="/ui-config" element={<UISettingsPage />} />
              <Route path="/twitch/settings" element={<TwitchSettingsPage />} />
              <Route
                path="/twitch/bot/commands"
                element={<TwitchBotCommandsPage />}
              />
              <Route
                path="/twitch/bot/timers"
                element={<TwitchBotTimersPage />}
              />
              <Route path="/twitch/bot/alerts" element={<ChatAlertsPage />} />
              <Route path="/loyalty/settings" element={<LoyaltyConfigPage />} />
              <Route path="/loyalty/users" element={<LoyaltyQueuePage />} />
              <Route path="/loyalty/rewards" element={<LoyaltyRewardsPage />} />
            </Routes>
          </PageWrapper>
        </PageContent>
      </Scrollbar>
    </Container>
  );
}
