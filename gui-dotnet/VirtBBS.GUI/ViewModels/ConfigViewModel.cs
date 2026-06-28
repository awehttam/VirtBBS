using System;
using System.Collections.Generic;
using System.Linq;
using System.Threading;
using System.Threading.Tasks;
using CommunityToolkit.Mvvm.ComponentModel;
using CommunityToolkit.Mvvm.Input;
using VirtBBS.GUI.Models;

namespace VirtBBS.GUI.ViewModels;

public partial class ConfigViewModel(ApiClient client) : ViewModelBase
{
    [ObservableProperty] private string _status = "";

    // BBS section.
    [ObservableProperty] private string _bbsName   = "";
    [ObservableProperty] private int    _maxNodes  = 10;

    // Network section.
    [ObservableProperty] private int    _telnetPort   = 2323;
    [ObservableProperty] private int    _sshPort      = 3232;
    [ObservableProperty] private int    _apiPort      = 9999;
    [ObservableProperty] private string _apiBind      = "0.0.0.0";
    [ObservableProperty] private int    _userApiPort  = 9998;
    [ObservableProperty] private string _userApiBind  = "0.0.0.0";
    [ObservableProperty] private int    _virtTermPort = 6323;
    [ObservableProperty] private string _virtTermBind = "0.0.0.0";
    [ObservableProperty] private int    _webPort      = 8081;
    [ObservableProperty] private string _webBind      = "0.0.0.0";

    // Paths section.
    [ObservableProperty] private string _dbPath     = "./data/virtbbs.db";
    [ObservableProperty] private string _filesPath  = "./files";
    [ObservableProperty] private string _logsPath   = "./logs";
    [ObservableProperty] private string _wwwPath    = "./www";

    // Session section.
    [ObservableProperty] private int _timePerCallMins = 60;
    [ObservableProperty] private int _idleTimeoutMins = 10;
    [ObservableProperty] private int _maxFailedLogins = 3;
    [ObservableProperty] private int _newUserSecurity = 10;

    // Sysop section.
    [ObservableProperty] private string _sysopName = "Sysop";

    [RelayCommand]
    public async Task LoadAsync(CancellationToken ct = default)
    {
        try
        {
            var cfg = await client.CallAsync<BbsConfig>("config.get", null, ct);
            if (cfg is null) return;

            BbsName         = cfg.Bbs.Name;
            MaxNodes        = cfg.Bbs.MaxNodes;
            TelnetPort      = cfg.Network.TelnetPort;
            SshPort         = cfg.Network.SshPort;
            ApiPort         = cfg.Network.ApiPort;
            ApiBind         = cfg.Network.ApiBind;
            UserApiPort     = cfg.Network.UserApiPort;
            UserApiBind     = cfg.Network.UserApiBind;
            VirtTermPort    = cfg.Network.VirtTermPort;
            VirtTermBind    = cfg.Network.VirtTermBind;
            WebPort         = cfg.Network.WebPort;
            WebBind         = cfg.Network.WebBind;
            DbPath          = cfg.Paths.Db;
            FilesPath       = cfg.Paths.Files;
            LogsPath        = cfg.Paths.Logs;
            WwwPath         = cfg.Paths.Www;
            TimePerCallMins = cfg.Session.TimePerCallMins;
            IdleTimeoutMins = cfg.Session.IdleTimeoutMins;
            MaxFailedLogins = cfg.Session.MaxFailedLogins;
            NewUserSecurity = cfg.Session.NewUserSecurity;
            SysopName       = cfg.Sysop.Name;

            Status = "Config loaded.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }

    [RelayCommand]
    private async Task SaveAsync(CancellationToken ct = default)
    {
        var patch = new
        {
            bbs     = new { name = BbsName,  max_nodes = MaxNodes },
            network = new {
                telnet_port = TelnetPort, ssh_port = SshPort, api_port = ApiPort, api_bind = ApiBind,
                userapi_port = UserApiPort, userapi_bind = UserApiBind,
                virtterm_port = VirtTermPort, virtterm_bind = VirtTermBind,
                web_port = WebPort, web_bind = WebBind,
            },
            paths   = new { db = DbPath, files = FilesPath, logs = LogsPath, www = WwwPath },
            session = new { time_per_call_mins = TimePerCallMins, idle_timeout_mins = IdleTimeoutMins,
                            max_failed_logins = MaxFailedLogins, new_user_security = NewUserSecurity },
            sysop   = new { name = SysopName },
        };
        try
        {
            await client.CallAsync("config.update", patch, ct);
            Status = "Config saved successfully.";
        }
        catch (Exception ex) { Status = $"Error: {ex.Message}"; }
    }
}
