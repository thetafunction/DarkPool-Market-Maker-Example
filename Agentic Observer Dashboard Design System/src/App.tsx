import React, { lazy, Suspense, useState } from 'react';
import { BootSequence } from './components/BootSequence';
import { PipBoyHeader } from './components/PipBoyHeader';
import { PipBoyTabs, TabType } from './components/PipBoyTabs';
import { PipBoyTerminal } from './components/PipBoyTerminal';
import { PipBoyDarkPool } from './components/PipBoyDarkPool';
import { PipBoyGauges } from './components/PipBoyGauges';
import { PipBoyFooter } from './components/PipBoyFooter';
import { PanelErrorBoundary } from './components/PanelErrorBoundary';

type ViewMode = 'RETRO' | 'NEURAL';

const NeuralNetwork = lazy(() =>
  import('./components/NeuralNetwork').then((mod) => ({ default: mod.NeuralNetwork })),
);
const HolographicOrderBook = lazy(() =>
  import('./components/HolographicOrderBook').then((mod) => ({ default: mod.HolographicOrderBook })),
);

const panelFallback = (label: string) => (
  <div className="pipboy-box p-6 h-full flex items-center justify-center">
    <div className="text-sm dim-text">{label}</div>
  </div>
);

export default function App() {
  const [bootComplete, setBootComplete] = useState(false);
  const [activeTab, setActiveTab] = useState<TabType>('EXEC');
  const [viewMode, setViewMode] = useState<ViewMode>('RETRO');

  if (!bootComplete) return <BootSequence onComplete={() => setBootComplete(true)} />;

  const isNeural = viewMode === 'NEURAL';
  const contentContainerStyle: React.CSSProperties = {
    minHeight: 0,
    display: 'flex',
    flexDirection: 'column',
  };

  return (
    <div
      className="min-h-screen text-foreground p-6 relative overflow-hidden"
      style={{ background: isNeural ? 'radial-gradient(circle at 20% 0%, #101c38 0%, #060913 45%, #020305 100%)' : '#000' }}
    >
      {!isNeural && <div className="grime-overlay"></div>}

      <div
        className="relative z-10"
        style={{ display: 'grid', gridTemplateRows: 'auto auto auto minmax(0,1fr) auto', minHeight: 'calc(100vh - 3rem)' }}
      >
        <PipBoyHeader />

        <div className="pipboy-box p-2 mb-4 flex items-center justify-between" style={isNeural ? { borderColor: '#2c6a99' } : undefined}>
          <div className="text-xs pixel-text" style={isNeural ? { color: '#9adfff' } : undefined}>VIEW MODE CONTROL</div>
          <div className="flex gap-2">
            <button
              onClick={() => setViewMode('RETRO')}
              className="px-6 py-3 border-2 text-xs font-mono tracking-wider transition"
              style={!isNeural ? { background: '#39ff14', borderColor: '#39ff14', color: '#000' } : { background: '#0a1c31', borderColor: '#2f6f9f', color: '#9fdbff' }}
            >
              WASTELAND
            </button>
            <button
              onClick={() => setViewMode('NEURAL')}
              className="px-6 py-3 border-2 text-xs font-mono tracking-wider transition"
              style={isNeural ? { background: '#58c4ff', borderColor: '#58c4ff', color: '#04101d' } : { background: '#001100', borderColor: '#008f11', color: '#39ff14' }}
            >
              NEURAL
            </button>
          </div>
        </div>

        <PipBoyTabs activeTab={activeTab} onTabChange={setActiveTab} />

        <div style={contentContainerStyle}>
          {activeTab === 'EXEC' && (
            <div className="grid grid-cols-12 gap-4 h-full">
              <div className="col-span-5 h-full">
                <PanelErrorBoundary panelName="Terminal">
                  <PipBoyTerminal />
                </PanelErrorBoundary>
              </div>
              <div className="col-span-7 h-full flex flex-col gap-4" style={{ minHeight: 0 }}>
                <div className="flex-1" style={{ minHeight: 0 }}>
                  <PanelErrorBoundary panelName={isNeural ? 'Neural Network' : 'Dark Pool'}>
                    {isNeural ? (
                      <Suspense fallback={panelFallback('Loading neural network...')}>
                        <NeuralNetwork />
                      </Suspense>
                    ) : (
                      <PipBoyDarkPool />
                    )}
                  </PanelErrorBoundary>
                </div>
                <div className="h-full">
                  <PanelErrorBoundary panelName={isNeural ? 'Holographic Order Book' : 'Gauges'}>
                    {isNeural ? (
                      <Suspense fallback={panelFallback('Loading order book...')}>
                        <HolographicOrderBook />
                      </Suspense>
                    ) : (
                      <PipBoyGauges />
                    )}
                  </PanelErrorBoundary>
                </div>
              </div>
            </div>
          )}

          {activeTab === 'VAULT' && (
            <div className="h-full">
              <PanelErrorBoundary panelName={isNeural ? 'Holographic Order Book' : 'Dark Pool'}>
                {isNeural ? (
                  <Suspense fallback={panelFallback('Loading order book...')}>
                    <HolographicOrderBook />
                  </Suspense>
                ) : (
                  <PipBoyDarkPool />
                )}
              </PanelErrorBoundary>
            </div>
          )}

          {activeTab === 'EFF' && (
            <div className="h-full flex items-center justify-center">
              <div className="w-full max-w-4xl">
                <PanelErrorBoundary panelName={isNeural ? 'Holographic Order Book' : 'Gauges'}>
                  {isNeural ? (
                    <Suspense fallback={panelFallback('Loading order book...')}>
                      <HolographicOrderBook />
                    </Suspense>
                  ) : (
                    <PipBoyGauges />
                  )}
                </PanelErrorBoundary>
              </div>
            </div>
          )}

          {activeTab === 'NET' && (
            <div className="h-full">
              <PanelErrorBoundary panelName={isNeural ? 'Neural Network' : 'Dark Pool'}>
                {isNeural ? (
                  <Suspense fallback={panelFallback('Loading neural network...')}>
                    <NeuralNetwork />
                  </Suspense>
                ) : (
                  <PipBoyDarkPool />
                )}
              </PanelErrorBoundary>
            </div>
          )}

          {activeTab === 'SYS' && (
            <div className="h-full pipboy-box p-8" style={isNeural ? { borderColor: '#3b6699' } : undefined}>
              <div className="grid grid-cols-2 gap-4 h-full">
                <div className="pipboy-box p-4 bg-[#000000]">
                  <div className="text-sm radioactive-text pixel-text mb-4 border-b border-[#008F11] pb-2">SYSTEM HEALTH</div>
                  <div className="space-y-3 text-xs">
                    <div className="flex justify-between"><span className="dim-text">CPU_USAGE:</span><span className="radioactive-text">23%</span></div>
                    <div className="flex justify-between"><span className="dim-text">TEE_STATUS:</span><span className="radioactive-text">ACTIVE</span></div>
                    <div className="flex justify-between"><span className="dim-text">UPTIME:</span><span className="radioactive-text">99.97%</span></div>
                  </div>
                </div>
                <div className="pipboy-box p-4 bg-[#000000]">
                  <div className="text-sm radioactive-text pixel-text mb-4 border-b border-[#008F11] pb-2">AGENT STATISTICS</div>
                  <div className="space-y-3 text-xs">
                    <div className="flex justify-between"><span className="dim-text">ACTIVE_AGENTS:</span><span className="radioactive-text">234</span></div>
                    <div className="flex justify-between"><span className="dim-text">TOTAL_TRADES:</span><span className="radioactive-text">1,247</span></div>
                    <div className="flex justify-between"><span className="dim-text">AUTONOMY:</span><span className="radioactive-text">MAXIMUM</span></div>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>

        <PipBoyFooter />
      </div>

      {!isNeural && <div className="fixed inset-0 pointer-events-none z-0 bg-gradient-radial from-[#39FF14]/5 via-transparent to-transparent blur-3xl"></div>}
      {isNeural && <div className="fixed inset-0 pointer-events-none z-0" style={{ background: 'radial-gradient(circle at 70% 10%, rgba(82, 198, 255, 0.16), transparent 35%), radial-gradient(circle at 20% 80%, rgba(167, 104, 255, 0.12), transparent 40%)' }}></div>}
    </div>
  );
}
