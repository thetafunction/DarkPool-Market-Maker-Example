import React, { useEffect, useMemo, useState } from 'react';
import { motion } from 'motion/react';

type Level = {
  price: number;
  qty: number;
};

type Snapshot = {
  bids: Level[];
  asks: Level[];
  spread: number;
  whaleSide: 'BID' | 'ASK';
  whaleSize: number;
};

const LEVELS = 16;

function randomAround(base: number, variance: number) {
  return base + (Math.random() - 0.5) * variance;
}

function createSnapshot(midPrice: number): Snapshot {
  const bids: Level[] = [];
  const asks: Level[] = [];

  for (let i = 0; i < LEVELS; i += 1) {
    const distance = (i + 1) * 0.35;
    const bidNoise = Math.random() * 20;
    const askNoise = Math.random() * 20;

    bids.push({
      price: Number((midPrice - distance).toFixed(2)),
      qty: Math.round(18 + bidNoise + (LEVELS - i) * 1.4),
    });

    asks.push({
      price: Number((midPrice + distance).toFixed(2)),
      qty: Math.round(18 + askNoise + (LEVELS - i) * 1.4),
    });
  }

  const whaleSide = Math.random() > 0.5 ? 'BID' : 'ASK';
  const whaleIndex = Math.floor(Math.random() * 4);
  const whaleSize = Math.round(95 + Math.random() * 120);

  if (whaleSide === 'BID') bids[whaleIndex].qty = whaleSize;
  if (whaleSide === 'ASK') asks[whaleIndex].qty = whaleSize;

  return {
    bids,
    asks,
    spread: Number((asks[0].price - bids[0].price).toFixed(2)),
    whaleSide,
    whaleSize,
  };
}

export const HolographicOrderBook: React.FC = () => {
  const [midPrice, setMidPrice] = useState(412.25);
  const [snapshot, setSnapshot] = useState<Snapshot>(() => createSnapshot(412.25));
  const [whalePulse, setWhalePulse] = useState(false);

  useEffect(() => {
    const interval = setInterval(() => {
      setMidPrice((prevMid) => {
        const nextMid = Number(randomAround(prevMid, 1.3).toFixed(2));
        const nextSnap = createSnapshot(nextMid);
        setSnapshot(nextSnap);
        setWhalePulse(nextSnap.whaleSize >= 160);
        return nextMid;
      });
    }, 1400);

    return () => clearInterval(interval);
  }, []);

  useEffect(() => {
    if (!whalePulse) return;
    const t = setTimeout(() => setWhalePulse(false), 900);
    return () => clearTimeout(t);
  }, [whalePulse]);

  const maxQty = useMemo(() => {
    return Math.max(
      ...snapshot.bids.map((i) => i.qty),
      ...snapshot.asks.map((i) => i.qty),
      1,
    );
  }, [snapshot]);

  return (
    <div
      className="pipboy-box p-4 h-full flex flex-col"
      style={{ borderColor: '#7f5bff', boxShadow: '0 0 18px rgba(137,95,255,0.35)' }}
    >
      <div className="mb-3 flex items-center justify-between">
        <div className="text-sm pixel-text" style={{ color: '#c9b8ff', textShadow: '0 0 10px rgba(175,132,255,0.8)' }}>
          HOLOGRAPHIC DEPTH FLOW
        </div>
        <div className="text-xs" style={{ color: '#8ad6ff' }}>
          Spread {snapshot.spread} DTH
        </div>
      </div>

      {whalePulse && (
        <motion.div
          initial={{ opacity: 0, y: -12 }}
          animate={{ opacity: 1, y: 0 }}
          className="mb-2 text-xs p-2"
          style={{ color: '#ffe9a8', background: 'rgba(255, 184, 77, 0.12)', border: '1px solid rgba(255, 184, 77, 0.45)' }}
        >
          WHALE ALERT: {snapshot.whaleSide} WALL {snapshot.whaleSize} DTH
        </motion.div>
      )}

      <div className="grid grid-cols-2 gap-4 flex-1 overflow-hidden">
        <div className="h-full overflow-hidden">
          <div className="text-xs mb-2" style={{ color: '#84ffc8' }}>BID PRESSURE</div>
          <div className="h-full overflow-hidden">
            {snapshot.bids.map((level, idx) => {
              const width = Math.max(8, (level.qty / maxQty) * 100);
              return (
                <motion.div
                  key={`bid-${level.price}`}
                  className="mb-1 p-1 text-xs flex items-center justify-between"
                  initial={{ opacity: 0.3, x: -8 }}
                  animate={{ opacity: 1, x: 0 }}
                  transition={{ duration: 0.3, delay: idx * 0.01 }}
                  style={{
                    width: `${width}%`,
                    background: 'linear-gradient(90deg, rgba(0,255,157,0.35), rgba(0,255,157,0.1))',
                    borderLeft: '2px solid rgba(0,255,157,0.7)',
                    color: '#acffe2',
                  }}
                >
                  <span>{level.price.toFixed(2)}</span>
                  <span>{level.qty}</span>
                </motion.div>
              );
            })}
          </div>
        </div>

        <div className="h-full overflow-hidden">
          <div className="text-xs mb-2" style={{ color: '#ff9fc9' }}>ASK PRESSURE</div>
          <div className="h-full overflow-hidden">
            {snapshot.asks.map((level, idx) => {
              const width = Math.max(8, (level.qty / maxQty) * 100);
              return (
                <motion.div
                  key={`ask-${level.price}`}
                  className="mb-1 p-1 text-xs flex items-center justify-between"
                  initial={{ opacity: 0.3, x: 8 }}
                  animate={{ opacity: 1, x: 0 }}
                  transition={{ duration: 0.3, delay: idx * 0.01 }}
                  style={{
                    marginLeft: `${100 - width}%`,
                    width: `${width}%`,
                    background: 'linear-gradient(270deg, rgba(255,117,170,0.35), rgba(255,117,170,0.1))',
                    borderRight: '2px solid rgba(255,117,170,0.7)',
                    color: '#ffd1e4',
                  }}
                >
                  <span>{level.qty}</span>
                  <span>{level.price.toFixed(2)}</span>
                </motion.div>
              );
            })}
          </div>
        </div>
      </div>

      <div className="mt-3 text-xs flex justify-between" style={{ color: '#9cb2ff' }}>
        <span>Mid: {midPrice.toFixed(2)} DTH</span>
        <span>Liquidity: {snapshot.bids[0].qty + snapshot.asks[0].qty}</span>
        <span>Cadence: 1.4s</span>
      </div>
    </div>
  );
};
