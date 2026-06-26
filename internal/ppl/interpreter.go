// ============================================================================
// VirtBBS — A modern BBS server inspired by PCBoard BBS
//           (Clark Development Company, 1987-1996)
//
// Copyright (c) 2026 John Dovey <dovey.john@gmail.com>
//
// MIT License
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS
// OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.
//
// Change History:
//   v0.0.1  2026-06-24  Initial implementation
// ============================================================================

package ppl

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

// signal types used for control flow
type signalKind int

const (
	sigNone signalKind = iota
	sigGoto
	sigGosub
	sigReturn
	sigEnd
	sigQuit
)

type signal struct {
	kind  signalKind
	label string // for goto/gosub
}

// Environment is the I/O and BBS context passed into a PPL program.
type Environment struct {
	// Output: write text to the user
	Print func(s string)
	// Input: read a line from the user (shown with optional prompt)
	Input func(prompt string) string
	// ReadKey: read a single keypress (no echo)
	ReadKey func() byte
	// DisplayFile: display a .ANS/.PCB display file
	DisplayFile func(path string)
	// Hangup: disconnect the user
	Hangup func()
	// User fields (readable/writable)
	UserName     string
	UserCity     string
	UserSec      int
	UserTimesOn  int
	UserMailWait bool
	// Lifetime user file/connection stats (from users DB).
	UserUploads         int
	UserDownloads       int
	UserBytesUploaded   int64
	UserBytesDownloaded int64
	UserLastLoginDate   string
	UserLastLoginTime   string
	// This-session activity (reset each call; written to callers log at logoff).
	SessMsgsRead  int
	SessMsgsLeft  int
	SessFilesDown int
	SessFilesUp   int
	SessMinutes   int
	SessTimeLeft  int
	// BBS-wide and per-user message stats.
	NewMsgsTotal   int
	BBSCallsToday  int
	BBSUniqueToday int
	BBSMsgTotal    int
	BBSConfCount   int
	BBSFileTotal   int
	BBSFileToday   int
	BBSFileMonth   int
	// BBS info
	BBSName      string
	SysopName    string
	NodeNum      int
	// File path for PPE (used to resolve relative paths)
	PPEPath string
}

// Interpreter executes a PPL AST.
type Interpreter struct {
	prog    *Program
	env     *Environment
	vars    map[string]Value   // global variables
	arrays  map[string][]Value // array variables
	procs   map[string]*ProcDecl
	funcs   map[string]*FuncDecl
	labels  map[string]int     // label → stmt index in current block
	callStack []frame
	openFiles map[int]*os.File
	rng     *rand.Rand
}

type frame struct {
	vars   map[string]Value
	arrays map[string][]Value
	ret    int // return address (stmt index) in parent
}

func NewInterpreter(prog *Program, env *Environment) *Interpreter {
	interp := &Interpreter{
		prog:      prog,
		env:       env,
		vars:      make(map[string]Value),
		arrays:    make(map[string][]Value),
		procs:     make(map[string]*ProcDecl),
		funcs:     make(map[string]*FuncDecl),
		labels:    make(map[string]int),
		openFiles: make(map[int]*os.File),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	// Pre-register procedures and functions; index labels
	for i, node := range prog.Stmts {
		switch n := node.(type) {
		case *ProcDecl:
			interp.procs[n.Name] = n
		case *FuncDecl:
			interp.funcs[n.Name] = n
		case *LabelStmt:
			interp.labels[strings.ToUpper(n.Name)] = i
		}
	}
	return interp
}

// Run executes the program.
func (interp *Interpreter) Run() error {
	_, err := interp.execStmts(interp.prog.Stmts)
	return err
}

func (interp *Interpreter) execStmts(stmts []Node) (signal, error) {
	i := 0
	for i < len(stmts) {
		sig, err := interp.execStmt(stmts[i])
		if err != nil {
			return signal{kind: sigEnd}, err
		}
		switch sig.kind {
		case sigEnd, sigQuit:
			return sig, nil
		case sigReturn:
			return sig, nil
		case sigGoto:
			// find label in current stmts
			found := false
			for j, s := range stmts {
				if l, ok := s.(*LabelStmt); ok && strings.ToUpper(l.Name) == strings.ToUpper(sig.label) {
					i = j + 1
					found = true
					break
				}
			}
			if !found {
				return sig, nil // bubble up
			}
			continue
		case sigGosub:
			// find label and execute from there, then return
			found := false
			for j, s := range stmts {
				if l, ok := s.(*LabelStmt); ok && strings.ToUpper(l.Name) == strings.ToUpper(sig.label) {
					sub := stmts[j+1:]
					subSig, err := interp.execStmts(sub)
					if err != nil {
						return subSig, err
					}
					found = true
					break
				}
			}
			if !found {
				return sig, nil
			}
		}
		i++
	}
	return signal{kind: sigNone}, nil
}

func (interp *Interpreter) execStmt(node Node) (signal, error) {
	switch n := node.(type) {
	case *LabelStmt:
		// no-op at execution time
		return signal{kind: sigNone}, nil

	case *DimStmt:
		if len(n.Dims) == 0 {
			interp.setVar(n.Name, zeroValue(n.Type))
		} else {
			size := 1
			for _, d := range n.Dims {
				v, err := interp.evalExpr(d)
				if err != nil {
					return signal{kind: sigEnd}, err
				}
				size *= int(v.ToInt()) + 1
			}
			arr := make([]Value, size)
			for i := range arr {
				arr[i] = zeroValue(n.Type)
			}
			interp.arrays[n.Name] = arr
		}
		return signal{kind: sigNone}, nil

	case *AssignStmt:
		val, err := interp.evalExpr(n.Value)
		if err != nil {
			return signal{kind: sigEnd}, err
		}
		if len(n.Index) == 0 {
			interp.setVar(n.Name, val)
		} else {
			idx, err := interp.evalExpr(n.Index[0])
			if err != nil {
				return signal{kind: sigEnd}, err
			}
			arr := interp.arrays[n.Name]
			i := int(idx.ToInt())
			if i < 0 || i >= len(arr) {
				return signal{kind: sigEnd}, fmt.Errorf("array index %d out of bounds for %s", i, n.Name)
			}
			arr[i] = val
		}
		return signal{kind: sigNone}, nil

	case *IfStmt:
		cond, err := interp.evalExpr(n.Cond)
		if err != nil {
			return signal{kind: sigEnd}, err
		}
		if cond.ToBool() {
			return interp.execStmts(n.Then)
		}
		for _, ei := range n.ElseIfs {
			cv, err := interp.evalExpr(ei.Cond)
			if err != nil {
				return signal{kind: sigEnd}, err
			}
			if cv.ToBool() {
				return interp.execStmts(ei.Stmts)
			}
		}
		if n.Else != nil {
			return interp.execStmts(n.Else)
		}
		return signal{kind: sigNone}, nil

	case *WhileStmt:
		for {
			cond, err := interp.evalExpr(n.Cond)
			if err != nil {
				return signal{kind: sigEnd}, err
			}
			if !cond.ToBool() {
				break
			}
			sig, err := interp.execStmts(n.Body)
			if err != nil || sig.kind != sigNone {
				return sig, err
			}
		}
		return signal{kind: sigNone}, nil

	case *ForStmt:
		startV, _ := interp.evalExpr(n.Start)
		stopV, _ := interp.evalExpr(n.Stop)
		stepV, _ := interp.evalExpr(n.Step)
		i := startV.ToInt()
		stop := stopV.ToInt()
		step := stepV.ToInt()
		if step == 0 {
			step = 1
		}
		for {
			if step > 0 && i > stop {
				break
			}
			if step < 0 && i < stop {
				break
			}
			interp.setVar(n.Var, IntVal(i))
			sig, err := interp.execStmts(n.Body)
			if err != nil || sig.kind != sigNone {
				return sig, err
			}
			i += step
		}
		return signal{kind: sigNone}, nil

	case *GotoStmt:
		return signal{kind: sigGoto, label: n.Label}, nil

	case *GosubStmt:
		return signal{kind: sigGosub, label: n.Label}, nil

	case *ReturnStmt:
		return signal{kind: sigReturn}, nil

	case *EndStmt:
		return signal{kind: sigEnd}, nil

	case *QuitStmt:
		return signal{kind: sigQuit}, nil

	case *ProcDecl, *FuncDecl:
		// declarations are pre-registered; skip at execution time
		return signal{kind: sigNone}, nil

	case *CallStmt:
		return interp.execCall(n)
	}
	return signal{kind: sigNone}, nil
}

func (interp *Interpreter) execCall(n *CallStmt) (signal, error) {
	// Evaluate all arguments
	args := make([]Value, len(n.Args))
	for i, a := range n.Args {
		v, err := interp.evalExpr(a)
		if err != nil {
			return signal{kind: sigEnd}, err
		}
		args[i] = v
	}

	// Built-in statements
	switch n.Name {
	case "PRINT", "PRINTLN", "PRINTLOC":
		var sb strings.Builder
		for _, a := range args {
			sb.WriteString(a.ToString())
		}
		text := sb.String()
		if n.Name == "PRINTLN" {
			text += "\r\n"
		}
		interp.env.Print(text)

	case "NEWLINE":
		times := int64(1)
		if len(args) > 0 {
			times = args[0].ToInt()
		}
		for i := int64(0); i < times; i++ {
			interp.env.Print("\r\n")
		}

	case "CLS":
		interp.env.Print("\x1b[2J\x1b[H")

	case "DISPFILE":
		if len(args) > 0 {
			interp.env.DisplayFile(args[0].ToString())
		}

	case "HANGUP":
		interp.env.Hangup()
		return signal{kind: sigQuit}, nil

	case "END", "QUIT", "STOP":
		return signal{kind: sigQuit}, nil

	case "INPUTSTR", "INPUT":
		prompt := ""
		varName := ""
		if n.Name == "INPUTSTR" && len(args) >= 2 {
			varName = n.Args[0].(*VarExpr).Name
			prompt = args[1].ToString()
		} else if len(args) >= 1 {
			if ve, ok := n.Args[0].(*VarExpr); ok {
				varName = ve.Name
				if len(args) > 1 {
					prompt = args[1].ToString()
				}
			}
		}
		if varName != "" {
			val := interp.env.Input(prompt)
			interp.setVar(varName, StrVal(val))
		}

	case "INPUTINT":
		varName, prompt := "", ""
		if len(n.Args) >= 1 {
			if ve, ok := n.Args[0].(*VarExpr); ok {
				varName = ve.Name
			}
		}
		if len(args) >= 2 {
			prompt = args[1].ToString()
		}
		if varName != "" {
			s := interp.env.Input(prompt)
			n, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			interp.setVar(varName, IntVal(n))
		}

	case "GETUSER":
		interp.setVar("U_NAME", StrVal(interp.env.UserName))
		interp.setVar("U_CITY", StrVal(interp.env.UserCity))
		interp.setVar("U_SEC", IntVal(int64(interp.env.UserSec)))
		interp.setVar("U_TIMESON", IntVal(int64(interp.env.UserTimesOn)))
		interp.setVar("U_MAILW", BoolVal(interp.env.UserMailWait))

	case "GETSTATS":
		e := interp.env
		interp.setVar("U_UPLOADS", IntVal(int64(e.UserUploads)))
		interp.setVar("U_DOWNLOADS", IntVal(int64(e.UserDownloads)))
		interp.setVar("U_KUP", IntVal(e.UserBytesUploaded/1024))
		interp.setVar("U_KDOWN", IntVal(e.UserBytesDownloaded/1024))
		interp.setVar("U_LASTDATE", StrVal(e.UserLastLoginDate))
		interp.setVar("U_LASTTIME", StrVal(e.UserLastLoginTime))
		interp.setVar("S_MSGREAD", IntVal(int64(e.SessMsgsRead)))
		interp.setVar("S_MSGLEFT", IntVal(int64(e.SessMsgsLeft)))
		interp.setVar("S_FILEDOWN", IntVal(int64(e.SessFilesDown)))
		interp.setVar("S_FILEUP", IntVal(int64(e.SessFilesUp)))
		interp.setVar("S_TIMEON", IntVal(int64(e.SessMinutes)))
		interp.setVar("S_TIMELEFT", IntVal(int64(e.SessTimeLeft)))
		interp.setVar("U_NEWMSG", IntVal(int64(e.NewMsgsTotal)))
		interp.setVar("BBS_TODAYCALLS", IntVal(int64(e.BBSCallsToday)))
		interp.setVar("BBS_TODAYUNIQUE", IntVal(int64(e.BBSUniqueToday)))
		interp.setVar("BBS_MSGS", IntVal(int64(e.BBSMsgTotal)))
		interp.setVar("BBS_CONFS", IntVal(int64(e.BBSConfCount)))
		interp.setVar("BBS_FILETOTAL", IntVal(int64(e.BBSFileTotal)))
		interp.setVar("BBS_FILETODAY", IntVal(int64(e.BBSFileToday)))
		interp.setVar("BBS_FILEMONTH", IntVal(int64(e.BBSFileMonth)))

	case "PUTUSER":
		interp.env.UserName = interp.getVar("U_NAME").ToString()
		interp.env.UserCity = interp.getVar("U_CITY").ToString()

	case "KBDSTUFF":
		// Stuff keyboard buffer — no-op in VirtBBS

	case "LOG":
		if len(args) > 0 {
			// Write to log — just print as sysop-visible text
			interp.env.Print("[LOG] " + args[0].ToString() + "\r\n")
		}

	case "DELAY":
		if len(args) > 0 {
			ms := args[0].ToInt() * 100 // PCBoard DELAY is in 1/10 sec
			if ms > 5000 {
				ms = 5000
			}
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}

	default:
		// User-defined procedure
		if proc, ok := interp.procs[n.Name]; ok {
			return interp.callProc(proc, args)
		}
		// Unknown — silently ignore for compatibility
	}
	return signal{kind: sigNone}, nil
}

func (interp *Interpreter) callProc(proc *ProcDecl, args []Value) (signal, error) {
	// Save variables, bind params
	saved := interp.vars
	interp.vars = make(map[string]Value)
	for k, v := range saved {
		interp.vars[k] = v
	}
	for i, param := range proc.Params {
		if i < len(args) {
			interp.vars[param.Name] = args[i]
		}
	}
	sig, err := interp.execStmts(proc.Body)
	// Restore variables (pass-by-value semantics)
	interp.vars = saved
	if sig.kind == sigReturn {
		return signal{kind: sigNone}, err
	}
	return sig, err
}

// ── Expression evaluator ──────────────────────────────────────────────────────

func (interp *Interpreter) evalExpr(e Expr) (Value, error) {
	switch n := e.(type) {
	case *IntLit:
		return IntVal(n.Val), nil
	case *RealLit:
		return RealVal(n.Val), nil
	case *StrLit:
		return StrVal(n.Val), nil
	case *BoolLit:
		return BoolVal(n.Val), nil

	case *VarExpr:
		if len(n.Index) > 0 {
			idx, _ := interp.evalExpr(n.Index[0])
			arr := interp.arrays[n.Name]
			i := int(idx.ToInt())
			if i >= 0 && i < len(arr) {
				return arr[i], nil
			}
			return Zero, nil
		}
		return interp.getVar(n.Name), nil

	case *UnaryExpr:
		v, err := interp.evalExpr(n.Val)
		if err != nil {
			return Zero, err
		}
		switch n.Op {
		case "-":
			if v.typ == TypeReal {
				return RealVal(-v.fnum), nil
			}
			return IntVal(-v.ToInt()), nil
		case "NOT":
			return BoolVal(!v.ToBool()), nil
		}

	case *BinExpr:
		return interp.evalBin(n)

	case *CallExpr:
		return interp.evalFuncCall(n)
	}
	return Zero, nil
}

func (interp *Interpreter) evalBin(n *BinExpr) (Value, error) {
	left, err := interp.evalExpr(n.Left)
	if err != nil {
		return Zero, err
	}
	// Short-circuit
	if n.Op == "AND" && !left.ToBool() {
		return False, nil
	}
	if n.Op == "OR" && left.ToBool() {
		return True, nil
	}
	right, err := interp.evalExpr(n.Right)
	if err != nil {
		return Zero, err
	}

	// String concatenation
	if n.Op == "+" && (left.typ == TypeString || left.typ == TypeBigStr) {
		return StrVal(left.ToString() + right.ToString()), nil
	}

	// Numeric ops — use float if either side is float
	if left.typ == TypeReal || right.typ == TypeReal {
		l, r := left.ToFloat(), right.ToFloat()
		switch n.Op {
		case "+":
			return RealVal(l + r), nil
		case "-":
			return RealVal(l - r), nil
		case "*":
			return RealVal(l * r), nil
		case "/":
			if r == 0 {
				return RealVal(0), nil
			}
			return RealVal(l / r), nil
		case "^":
			return RealVal(math.Pow(l, r)), nil
		case "=":
			return BoolVal(l == r), nil
		case "<>", "!=":
			return BoolVal(l != r), nil
		case "<":
			return BoolVal(l < r), nil
		case "<=":
			return BoolVal(l <= r), nil
		case ">":
			return BoolVal(l > r), nil
		case ">=":
			return BoolVal(l >= r), nil
		}
	}

	// Integer ops
	l, r := left.ToInt(), right.ToInt()
	switch n.Op {
	case "+":
		return IntVal(l + r), nil
	case "-":
		return IntVal(l - r), nil
	case "*":
		return IntVal(l * r), nil
	case "/":
		if r == 0 {
			return IntVal(0), nil
		}
		return IntVal(l / r), nil
	case "^":
		return IntVal(int64(math.Pow(float64(l), float64(r)))), nil
	case "MOD", "%":
		if r == 0 {
			return IntVal(0), nil
		}
		return IntVal(l % r), nil
	case "=":
		// String equality if both are strings
		if left.typ == TypeString || left.typ == TypeBigStr {
			return BoolVal(strings.EqualFold(left.str, right.str)), nil
		}
		return BoolVal(l == r), nil
	case "<>", "!=":
		if left.typ == TypeString || left.typ == TypeBigStr {
			return BoolVal(!strings.EqualFold(left.str, right.str)), nil
		}
		return BoolVal(l != r), nil
	case "<":
		return BoolVal(l < r), nil
	case "<=":
		return BoolVal(l <= r), nil
	case ">":
		return BoolVal(l > r), nil
	case ">=":
		return BoolVal(l >= r), nil
	case "AND":
		return BoolVal(left.ToBool() && right.ToBool()), nil
	case "OR":
		return BoolVal(left.ToBool() || right.ToBool()), nil
	}
	return Zero, nil
}

func (interp *Interpreter) evalFuncCall(n *CallExpr) (Value, error) {
	args := make([]Value, len(n.Args))
	for i, a := range n.Args {
		v, err := interp.evalExpr(a)
		if err != nil {
			return Zero, err
		}
		args[i] = v
	}

	arg0str := func() string {
		if len(args) > 0 {
			return args[0].ToString()
		}
		return ""
	}
	arg0int := func() int64 {
		if len(args) > 0 {
			return args[0].ToInt()
		}
		return 0
	}

	switch n.Name {
	// String functions
	case "LEN":
		return IntVal(int64(len(arg0str()))), nil
	case "LEFT":
		s := arg0str()
		ln := int(args[1].ToInt())
		if ln > len(s) {
			ln = len(s)
		}
		return StrVal(s[:ln]), nil
	case "RIGHT":
		s := arg0str()
		ln := int(args[1].ToInt())
		if ln > len(s) {
			ln = len(s)
		}
		return StrVal(s[len(s)-ln:]), nil
	case "MID":
		s := arg0str()
		start := int(args[1].ToInt()) - 1 // 1-based
		if start < 0 {
			start = 0
		}
		if start >= len(s) {
			return StrVal(""), nil
		}
		ln := len(s) - start
		if len(args) > 2 {
			ln = int(args[2].ToInt())
		}
		end := start + ln
		if end > len(s) {
			end = len(s)
		}
		return StrVal(s[start:end]), nil
	case "UPPER":
		return StrVal(strings.ToUpper(arg0str())), nil
	case "LOWER":
		return StrVal(strings.ToLower(arg0str())), nil
	case "TRIM":
		return StrVal(strings.TrimSpace(arg0str())), nil
	case "LTRIM":
		return StrVal(strings.TrimLeft(arg0str(), " ")), nil
	case "RTRIM":
		return StrVal(strings.TrimRight(arg0str(), " ")), nil
	case "CHR":
		return StrVal(string(rune(arg0int()))), nil
	case "ASC":
		s := arg0str()
		if len(s) == 0 {
			return IntVal(0), nil
		}
		return IntVal(int64(s[0])), nil
	case "STR", "STRING":
		return StrVal(args[0].ToString()), nil
	case "VAL", "INT":
		return IntVal(arg0int()), nil
	case "INSTR":
		if len(args) >= 2 {
			return IntVal(int64(strings.Index(args[0].ToString(), args[1].ToString()) + 1)), nil
		}
		return IntVal(0), nil
	case "SPACE":
		return StrVal(strings.Repeat(" ", int(arg0int()))), nil
	case "REPLICATE":
		if len(args) >= 2 {
			return StrVal(strings.Repeat(args[0].ToString(), int(args[1].ToInt()))), nil
		}
		return Empty, nil
	case "STRIP":
		return StrVal(strings.Map(func(r rune) rune {
			if r < 32 {
				return -1
			}
			return r
		}, arg0str())), nil

	// Math functions
	case "ABS":
		if args[0].typ == TypeReal {
			return RealVal(math.Abs(args[0].fnum)), nil
		}
		v := arg0int()
		if v < 0 {
			v = -v
		}
		return IntVal(v), nil
	case "SQR", "SQRT":
		return RealVal(math.Sqrt(args[0].ToFloat())), nil
	case "POW":
		if len(args) >= 2 {
			return RealVal(math.Pow(args[0].ToFloat(), args[1].ToFloat())), nil
		}
		return RealVal(0), nil
	case "MOD":
		if len(args) >= 2 && args[1].ToInt() != 0 {
			return IntVal(arg0int() % args[1].ToInt()), nil
		}
		return IntVal(0), nil
	case "RND":
		return RealVal(interp.rng.Float64()), nil
	case "RANDOM":
		if len(args) > 0 && args[0].ToInt() > 0 {
			return IntVal(interp.rng.Int63n(args[0].ToInt())), nil
		}
		return IntVal(0), nil

	// Date/time
	case "DATE":
		return StrVal(time.Now().Format("01-02-2006")), nil
	case "TIME":
		return StrVal(time.Now().Format("15:04")), nil

	// User variables
	case "U_NAME":
		return StrVal(interp.env.UserName), nil
	case "U_CITY":
		return StrVal(interp.env.UserCity), nil
	case "U_SEC", "CURSEC":
		return IntVal(int64(interp.env.UserSec)), nil
	case "U_TIMESON":
		return IntVal(int64(interp.env.UserTimesOn)), nil

	// BBS info
	case "BBSNAME":
		return StrVal(interp.env.BBSName), nil
	case "SYSOPNAME":
		return StrVal(interp.env.SysopName), nil
	case "NODENUM":
		return IntVal(int64(interp.env.NodeNum)), nil

	// File/system
	case "EXIST":
		_, err := os.Stat(arg0str())
		return BoolVal(err == nil), nil
	case "PPEPATH":
		return StrVal(interp.env.PPEPath), nil

	// Logical
	case "IIF":
		if len(args) >= 3 && args[0].ToBool() {
			return args[1], nil
		}
		if len(args) >= 3 {
			return args[2], nil
		}
		return False, nil

	// User-defined function
	default:
		if fn, ok := interp.funcs[n.Name]; ok {
			result, err := interp.callFunc(fn, args)
			return result, err
		}
	}
	return Zero, nil
}

func (interp *Interpreter) callFunc(fn *FuncDecl, args []Value) (Value, error) {
	saved := interp.vars
	interp.vars = make(map[string]Value)
	for k, v := range saved {
		interp.vars[k] = v
	}
	// Bind params
	for i, param := range fn.Params {
		if i < len(args) {
			interp.vars[param.Name] = args[i]
		}
	}
	// Return value stored in variable with function name
	interp.vars[fn.Name] = zeroValue(fn.ReturnType)
	_, err := interp.execStmts(fn.Body)
	result := interp.vars[fn.Name]
	interp.vars = saved
	return result, err
}

// ── Variable access ───────────────────────────────────────────────────────────

func (interp *Interpreter) getVar(name string) Value {
	name = strings.ToUpper(name)
	if v, ok := interp.vars[name]; ok {
		return v
	}
	return Zero
}

func (interp *Interpreter) setVar(name string, v Value) {
	interp.vars[strings.ToUpper(name)] = v
}

func zeroValue(tc TypeCode) Value {
	switch tc {
	case TypeBoolean:
		return False
	case TypeReal:
		return RealVal(0)
	case TypeString, TypeBigStr:
		return Empty
	default:
		return Zero
	}
}
