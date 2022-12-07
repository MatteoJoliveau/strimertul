import React, { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useModule, useStatus } from '../../lib/react-utils';
import { useAppDispatch } from '../../store';
import apiReducer, { modules } from '../../store/api/reducer';
import RevealLink from '../components/utils/RevealLink';
import SaveButton from '../components/utils/SaveButton';
import {
  APPNAME,
  Field,
  FieldNote,
  InputBox,
  Label,
  PageContainer,
  PageHeader,
  PageTitle,
  PasswordInputBox,
} from '../theme';

export default function ServerSettingsPage(): React.ReactElement {
  const [serverConfig, setServerConfig, loadStatus] = useModule(
    modules.httpConfig,
  );
  const { t } = useTranslation();
  const dispatch = useAppDispatch();
  const status = useStatus(loadStatus.save);
  const busy =
    loadStatus.load?.type !== 'success' || loadStatus.save?.type === 'pending';
  const [revealKVPassword, setRevealKVPassword] = useState(false);

  return (
    <PageContainer>
      <PageHeader css={{ paddingBottom: '1rem' }}>
        <PageTitle>{t('pages.http.title')}</PageTitle>
      </PageHeader>
      <form
        onSubmit={(ev) => {
          void dispatch(setServerConfig(serverConfig));
          ev.preventDefault();
        }}
      >
        <Field size="fullWidth">
          <Label htmlFor="bind">{t('pages.http.bind')}</Label>
          <InputBox
            type="text"
            id="bind"
            placeholder={t('pages.http.bind-placeholder')}
            value={serverConfig?.bind ?? ''}
            disabled={busy}
            required={true}
            onChange={(e) =>
              dispatch(
                apiReducer.actions.httpConfigChanged({
                  ...serverConfig,
                  bind: e.target.value,
                }),
              )
            }
          />
          <FieldNote>{t('pages.http.bind-help', { APPNAME })}</FieldNote>
        </Field>
        <Field size="fullWidth">
          <Label htmlFor="kvpassword">
            {t('pages.http.kilovolt-password')}
            <RevealLink value={revealKVPassword} setter={setRevealKVPassword} />
          </Label>{' '}
          <PasswordInputBox
            reveal={revealKVPassword}
            id="kvpassword"
            placeholder={t('pages.http.kilovolt-placeholder')}
            value={serverConfig?.kv_password ?? ''}
            disabled={busy}
            autoComplete="off"
            onChange={(e) => {
              dispatch(
                apiReducer.actions.httpConfigChanged({
                  ...serverConfig,
                  kv_password: e.target.value,
                }),
              );
            }}
          />
          <FieldNote>{t('pages.http.kilovolt-placeholder')}</FieldNote>
        </Field>
        <Field size="fullWidth">
          <Label htmlFor="static">{t('pages.http.static-path')}</Label>
          <InputBox
            type="text"
            id="static"
            placeholder={t('pages.http.static-placeholder')}
            disabled={busy}
            onChange={(e) =>
              dispatch(
                apiReducer.actions.httpConfigChanged({
                  ...serverConfig,
                  path: e.target.value,
                }),
              )
            }
            value={
              serverConfig?.enable_static_server ? serverConfig?.path ?? '' : ''
            }
          />
          <FieldNote>
            {t('pages.http.static-help', {
              url: `http://${serverConfig?.bind ?? 'localhost:4337'}/static/`,
            })}
          </FieldNote>
        </Field>
        <SaveButton type="submit" status={status} />
      </form>
    </PageContainer>
  );
}
