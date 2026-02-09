import React, { useState, useEffect } from 'react';
import { motion } from 'motion/react';
import { Shield, Lock } from 'lucide-react';

interface Particle {
  id: number;
  x: number;
  y: number;
  delay: number;
}

export const RetroVault: React.FC = () => {
  const [privacyShield, setPrivacyShield] = useState(87.4);
  const [activeAgents, setActiveAgents] = useState(234);
  const [teeOperations, setTeeOperations] = useState(1247);
  const [particles, setParticles] = useState<Particle[]>([]);

  useEffect(() => {
    // Generate particles
    const newParticles = Array.from({ length: 15 }, (_, i) => ({
      id: i,
      x: Math.random() * 100,
      y: 100 + Math.random() * 20,
      delay: Math.random() * 3
    }));
    setParticles(newParticles);

    const interval = setInterval(() => {
      setPrivacyShield(prev => {
        const change = (Math.random() - 0.5) * 2;
        return Math.max(75, Math.min(99, prev + change));
      });
      setActiveAgents(prev => prev + Math.floor(Math.random() * 3 - 1));
      setTeeOperations(prev => prev + Math.floor(Math.random() * 10));
    }, 2000);

    return () => clearInterval(interval);
  }, []);

  return (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="terminal-box p-3 mb-4">
        <div className="flex items-center justify-between">
          <div className="text-xs font-mono tracking-wider text-[#00FF41] uppercase flex items-center gap-2">
            <Lock className="w-4 h-4" strokeWidth={1} />
            SHIELDED_EXECUTION_SYSTEM
          </div>
          <div className="text-[9px] text-[#558855] font-mono">
            STATUS: <span className="text-[#CCFF00]">SHIELDED</span>
          </div>
        </div>
      </div>

      {/* Main Vault */}
      <div className="flex-1 terminal-box p-6 relative overflow-hidden">
        {/* Wireframe 3D Cube */}
        <div className="absolute inset-0 flex items-center justify-center">
          <svg
            viewBox="0 0 400 400"
            className="w-full h-full max-w-md max-h-md"
            style={{ filter: 'drop-shadow(0 0 10px rgba(0, 255, 65, 0.5))' }}
          >
            {/* Back face */}
            <polygon
              points="100,80 300,80 300,280 100,280"
              fill="none"
              stroke="#00FF41"
              strokeWidth="1"
              opacity="0.3"
            />
            
            {/* Front face */}
            <polygon
              points="140,120 340,120 340,320 140,320"
              fill="rgba(10, 15, 10, 0.8)"
              stroke="#00FF41"
              strokeWidth="1.5"
              className="animate-pulse-border"
            />
            
            {/* Connecting lines */}
            <line x1="100" y1="80" x2="140" y2="120" stroke="#00FF41" strokeWidth="1" opacity="0.5" />
            <line x1="300" y1="80" x2="340" y2="120" stroke="#00FF41" strokeWidth="1" opacity="0.5" />
            <line x1="300" y1="280" x2="340" y2="320" stroke="#00FF41" strokeWidth="1" opacity="0.5" />
            <line x1="100" y1="280" x2="140" y2="320" stroke="#00FF41" strokeWidth="1" opacity="0.5" />

            {/* Inner grid */}
            {Array.from({ length: 5 }).map((_, i) => (
              <React.Fragment key={`h-${i}`}>
                <line
                  x1="140"
                  y1={120 + i * 50}
                  x2="340"
                  y2={120 + i * 50}
                  stroke="#00FF41"
                  strokeWidth="0.5"
                  opacity="0.2"
                />
              </React.Fragment>
            ))}
            {Array.from({ length: 5 }).map((_, i) => (
              <React.Fragment key={`v-${i}`}>
                <line
                  x1={140 + i * 50}
                  y1="120"
                  x2={140 + i * 50}
                  y2="320"
                  stroke="#00FF41"
                  strokeWidth="0.5"
                  opacity="0.2"
                />
              </React.Fragment>
            ))}

            {/* Center lock icon area */}
            <circle cx="240" cy="220" r="30" fill="none" stroke="#CCFF00" strokeWidth="1.5" className="animate-digital-glow" />
            <circle cx="240" cy="220" r="20" fill="none" stroke="#CCFF00" strokeWidth="1" opacity="0.5" />
          </svg>

          {/* Center shield icon */}
          <div className="absolute inset-0 flex items-center justify-center">
            <Shield className="w-12 h-12 text-[#CCFF00] animate-digital-glow" strokeWidth={1} />
          </div>
        </div>

        {/* Ghost Particles */}
        {particles.map(particle => (
          <motion.div
            key={particle.id}
            className="absolute w-2 h-2 bg-[#00FF41] rounded-full"
            style={{
              left: `${particle.x}%`,
              bottom: '0%',
              boxShadow: '0 0 10px #00FF41'
            }}
            animate={{
              y: [-400, 0],
              opacity: [0, 1, 1, 0],
            }}
            transition={{
              duration: 4,
              delay: particle.delay,
              repeat: Infinity,
              ease: "linear"
            }}
          />
        ))}

        {/* Privacy Shield Bar */}
        <div className="absolute bottom-6 left-6 right-6">
          <div className="terminal-box p-3 bg-[#050805]">
            <div className="flex items-center justify-between mb-2">
              <div className="text-[10px] text-[#558855] font-mono tracking-widest">
                PRIVACY_SHIELD_INTEGRITY
              </div>
              <div className="text-sm font-mono segment-display text-[#CCFF00]">
                {privacyShield.toFixed(1)}%
              </div>
            </div>
            <div className="h-3 bg-[#0a0f0a] border border-[#00FF41]/30 relative overflow-hidden">
              <motion.div
                className="absolute inset-0 flex"
                style={{ width: `${privacyShield}%` }}
              >
                {Array.from({ length: 50 }).map((_, i) => (
                  <div
                    key={i}
                    className="flex-1 bg-[#CCFF00] border-r border-[#050805]"
                    style={{
                      opacity: 0.8 - (i * 0.015),
                      animation: `blink ${0.5 + Math.random()}s infinite`
                    }}
                  />
                ))}
              </motion.div>
            </div>
          </div>
        </div>

        {/* Stats */}
        <div className="absolute top-6 left-6 right-6 grid grid-cols-3 gap-3">
          <div className="terminal-box p-2 text-center bg-[#050805]">
            <div className="text-lg font-mono segment-display text-[#00FF41]">{activeAgents}</div>
            <div className="text-[9px] text-[#558855] font-mono tracking-wider mt-1">AGENTS</div>
          </div>
          <div className="terminal-box p-2 text-center bg-[#050805]">
            <div className="text-lg font-mono segment-display text-[#00FF41]">{teeOperations}</div>
            <div className="text-[9px] text-[#558855] font-mono tracking-wider mt-1">TEE_OPS</div>
          </div>
          <div className="terminal-box p-2 text-center bg-[#050805]">
            <div className="text-lg font-mono segment-display text-[#CCFF00]">0</div>
            <div className="text-[9px] text-[#558855] font-mono tracking-wider mt-1">EXPOSED</div>
          </div>
        </div>
      </div>

      {/* Bottom label */}
      <div className="terminal-box p-2 mt-4 text-center">
        <div className="text-[10px] text-[#558855] font-mono italic">
          &gt;&gt; THE_INVISIBLE_HAND: UNSEEN_LIQUIDITY_IN_MOTION
        </div>
      </div>
    </div>
  );
};
